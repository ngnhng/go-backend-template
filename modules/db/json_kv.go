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
package db

import (
	"context"
	"encoding/json"
	"fmt"
)

type (

	// JSONKV wraps a db.KV and transparently JSON-encodes/decodes values of type T.
	//
	// It uses AtomicGet/AtomicSet under the hood but exposes a typed API:
	//
	//   kv := NewRedisKV(client, WithKeyPrefix("profile:"))
	//   jsonKV := NewJSONKV[Profile](kv)
	//   prev, _ := jsonKV.Set(ctx, "user:123", profile)
	//   curr, _ := jsonKV.Get(ctx, "user:123")
	JSONKV[T any] struct {
		KV
	}
)

// NewJSONKV constructs a JSONKV wrapper on top of an existing db.KV.
func NewJSONKV[T any](kv KV) JSONKV[T] {
	return JSONKV[T]{KV: kv}
}

func (j JSONKV[T]) Get(ctx context.Context, key string) (*T, error) {
	raw, err := j.KV.AtomicGet(ctx, key)
	if err != nil {
		return nil, err
	}

	if raw == nil {
		return nil, nil
	}

	bs, ok := raw.([]byte)
	if !ok {
		return nil, fmt.Errorf("jsonkv: expected []byte for key %q, got %T", key, raw)
	}

	var v T
	if err := json.Unmarshal(bs, &v); err != nil {
		return nil, fmt.Errorf("jsonkv: decode %q: %w", key, err)
	}

	return &v, nil
}

// Set atomically sets key to value and returns the previous value (if any), decoded into T.
func (j JSONKV[T]) Set(ctx context.Context, key string, value T) (*T, error) {
	prev, err := j.KV.AtomicSet(ctx, key, value)
	if err != nil {
		return nil, err
	}
	if prev == nil {
		return nil, nil
	}

	bs, ok := prev.([]byte)
	if !ok {
		return nil, fmt.Errorf("jsonkv: expected []byte previous value for key %q, got %T", key, prev)
	}

	var v T
	if err := json.Unmarshal(bs, &v); err != nil {
		return nil, fmt.Errorf("jsonkv: decode previous value for key %q: %w", key, err)
	}

	return &v, nil
}
