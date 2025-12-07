// Copyright 2025 Nhat-Nguyen Nguyen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"app/modules/db"

	"github.com/redis/rueidis"
)

var (
	_ db.KV = (*RedisKV)(nil)

	//go:embed atomic_set.lua
	atomicSetLua string

	// TODO: fail fast if client does not have EVAL permission?
	//
	// Lua script for AtomicSet:
	//
	//   - KEYS[1]  = full key
	//   - ARGV[1]  = serialized value
	//   - ARGV[2]  = TTL in seconds (string; 0 or empty = no TTL)
	//
	// Atomically:
	//
	//	prev = GET key
	//	if ttl > 0 then SET key value EX ttl else SET key value end
	//	return prev
	//
	// This gives us:
	//   - Single round-trip
	//   - Atomic read-modify-write including TTL update
	luaAtomicSet = rueidis.NewLuaScript(atomicSetLua)
)

// RedisKV is a Rueidis-backed implementation of db.KV with:
//
//   - Key prefixing (multi-tenant / env scoping)
//   - AtomicSet via Lua (GET + SET + TTL in one script)
//   - Optional server-assisted client-side caching for reads (AtomicGet)
type RedisKV struct {
	client rueidis.Client

	// prefix is optional and should already end with ":" if non-empty.
	prefix string

	// defaultTTL is applied to every AtomicSet if > 0.
	defaultTTL time.Duration

	// If true, AtomicGet will use DoCache with cache TTL = defaultTTL.
	enableClientCache bool
}

// RedisKVOption configures RedisKV.
type RedisKVOption func(*RedisKV)

// WithKeyPrefix scopes all keys under a prefix (env, service, etc).
// Example: WithKeyPrefix("app:profile:dev") → keys "user:123" stored as "app:profile:dev:user:123".
func WithKeyPrefix(prefix string) RedisKVOption {
	return func(k *RedisKV) {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && !strings.HasSuffix(prefix, ":") {
			prefix += ":"
		}
		k.prefix = prefix
	}
}

// WithDefaultTTL configures a default TTL for all AtomicSet operations.
// A value <= 0 means "no TTL".
func WithDefaultTTL(ttl time.Duration) RedisKVOption {
	return func(k *RedisKV) {
		k.defaultTTL = ttl
	}
}

// WithClientSideCache enables server-assisted client-side caching for AtomicGet.
// You must also configure ClientTrackingPrefixes in RueidisOptions to match the prefix.
func WithClientSideCache() RedisKVOption {
	return func(k *RedisKV) {
		k.enableClientCache = true
	}
}

// NewRedisKV constructs a RedisKV on top of an existing rueidis.Client.
//
// The same client can be shared across multiple RedisKV instances (different prefixes).
func NewRedisKV(client rueidis.Client, opts ...RedisKVOption) *RedisKV {
	kv := &RedisKV{
		client: client,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(kv)
		}
	}
	return kv
}

// key builds the namespaced key.
func (k *RedisKV) key(raw string) string {
	if k.prefix == "" {
		return raw
	}
	return k.prefix + raw
}

// AtomicGet implements db.KV.AtomicGet.
//
//   - Returns []byte (as `any`) on success
//   - Returns (nil, nil) if the key does not exist
//   - Optionally uses server-assisted client-side caching if enabled
func (k *RedisKV) AtomicGet(ctx context.Context, key string) (any, error) {
	fullKey := k.key(key)

	var res rueidis.RedisResult
	if k.enableClientCache && k.defaultTTL > 0 {
		// Use L1 cache with TTL = defaultTTL
		res = k.client.DoCache(ctx, k.client.B().Get().Key(fullKey).Cache(), k.defaultTTL)
	} else {
		res = k.client.Do(ctx, k.client.B().Get().Key(fullKey).Build())
	}

	bs, err := res.AsBytes()
	if err != nil {
		if re, ok := rueidis.IsRedisErr(err); ok && re.IsNil() {
			// Key missing – treat as nil value.
			return nil, nil
		}
		return nil, fmt.Errorf("redis kv: AtomicGet %q failed: %w", key, err)
	}

	return bs, nil
}

// AtomicSet implements db.KV.AtomicSet.
//
//   - Serializes value (string / []byte / Stringer / JSON)
//
// Uses a Lua script to atomically:
//
//   - GET previous value
//
//   - SET new value (with EX TTL if configured)
//
//   - Returns the previous value as []byte or nil if none
func (r *RedisKV) AtomicSet(ctx context.Context, key string, value any) (any, error) {
	fullKey := r.key(key)

	serialized, err := encodeValue(value)
	if err != nil {
		return nil, fmt.Errorf("redis kv: encode value for key %q: %w", key, err)
	}

	ttlArg := ""
	if r.defaultTTL > 0 {
		ttl := int64(r.defaultTTL / time.Second)
		if ttl <= 0 {
			ttl = 1
		}
		ttlArg = strconv.FormatInt(ttl, 10)
	}

	// TODO: needs permission for EVAL beforehand, how to check/fail fast?
	res := luaAtomicSet.Exec(ctx, r.client, []string{fullKey}, []string{serialized, ttlArg})
	bs, err := res.AsBytes()
	if err != nil {
		if re, ok := rueidis.IsRedisErr(err); ok && re.IsNil() {
			// No previous value.
			return nil, nil
		}
		return nil, fmt.Errorf("redis kv: AtomicSet %q failed: %w", key, err)
	}

	// Return raw bytes so higher-level wrappers can decode (JSON, protobuf, etc).
	return bs, nil
}

// HealthCheck is a small helper to be used by readiness/liveness probes.
func (k *RedisKV) HealthCheck(ctx context.Context) error {
	return k.client.Do(ctx, k.client.B().Ping().Build()).Error()
}

// encodeValue serializes a value into a Redis string.
//
//   - string → as-is
//   - []byte → BinaryString (no extra alloc)
//   - fmt.Stringer → String()
//   - everything else → JSON
func encodeValue(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", errors.New("redis kv: nil values are not allowed")
	case string:
		return x, nil
	case []byte:
		return rueidis.BinaryString(x), nil
	case fmt.Stringer:
		return x.String(), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return rueidis.BinaryString(b), nil
	}
}
