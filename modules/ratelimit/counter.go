package ratelimit

import (
	"context"
	"time"
)

// TODO: on distributed count / counter service?

// CounterStore is the storage abstraction ratelimit uses.
type CounterStore interface {
	// Incr increments a counter at key and returns the new value.
	// TTL tells the store how long to keep the key alive (at least).
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)

	// Get returns the current value of a counter, or 0 if missing.
	Get(ctx context.Context, key string) (int64, error)
}
