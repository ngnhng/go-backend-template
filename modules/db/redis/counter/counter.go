// Copyright 2025 Nhat-Nguyen Nguyen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package counter

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"time"

	"app/modules/ratelimit"

	"github.com/redis/rueidis"
	_ "github.com/redis/rueidis/rueidishook"
)

var (
	_ ratelimit.CounterStore = (*RedisCounter)(nil)

	//go:embed incr_expr.lua
	atomicIncrLua string

	// Lua script for Atomic Increment
	// - KEYS[1] = full key
	// - ARGV[1] = TTL to be set for a new counter
	// Atomically:
	// - Key Count = Key Count + 1
	// - If count after INCR = 1, set EXPIRE for Key = TTL
	luaAtomicIncrWithTTL = rueidis.NewLuaScript(atomicIncrLua)
)

type RedisCounter struct {
	client rueidis.Client
	prefix string
}

// NewRedisCounterStore wraps a rueidis.Client as a CounterStore.
//
// prefix is optional; if non-empty, keys become prefix + ":" + key.
func NewRedisCounterStore(client rueidis.Client, prefix string) *RedisCounter {
	if prefix != "" && prefix[len(prefix)-1] != ':' {
		prefix += ":"
	}
	return &RedisCounter{
		client: client,
		prefix: prefix,
	}
}

// Optionally add hooks (logging, OTEL) via rueidishook here.
func NewInstrumentedRedisCounterStore(client rueidis.Client, prefix string) ratelimit.CounterStore {
	// hooked := rueidishook.WithHook(client, )
	// return NewRedisCounterStore(hooked, prefix)
	return NewRedisCounterStore(client, prefix)
}

func (r *RedisCounter) buildKey(key string) string {
	return r.prefix + key
}

// Get implements ratelimit.CounterStore.
func (r *RedisCounter) Get(ctx context.Context, key string) (int64, error) {
	k := r.buildKey(key)
	rr := r.client.Do(ctx, r.client.B().Get().Key(k).Build())
	bs, err := rr.AsBytes()
	if err != nil {
		// rueidis intentionally does not classify a NIL reply as a “Redis ERR”.
		if ret, ok := rueidis.IsRedisErr(err); ok && ret.IsNil() {
			return 0, nil
		} else if rueidis.IsRedisNil(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("redis counter Get: %w", err)
	}

	n, err := strconv.ParseInt(string(bs), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("redis counter Get parse: %w", err)
	}
	return n, nil
}

// Incr implements ratelimit.CounterStore.
func (r *RedisCounter) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	ms := ttl.Milliseconds()
	k := r.buildKey(key)

	// TODO: writing on how lua scripting is executed in redis
	rr := luaAtomicIncrWithTTL.Exec(ctx, r.client, []string{k}, []string{strconv.FormatInt(ms, 10)})
	val, err := rr.AsInt64()
	if err != nil {
		return 0, fmt.Errorf("redis counter Incr: %w", err)
	}
	return val, nil
}
