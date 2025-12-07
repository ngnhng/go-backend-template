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

package ratelimit

import (
	"context"
	"time"
)

// slidingWindowType: Can be COUNT_BASED or TIME_BASED.
// slidingWindowSize:
// For COUNT_BASED: The number of the last calls to record and aggregate (e.g., the last 100 calls).
// For TIME_BASED: The duration (e.g., in seconds) over which to record and aggregate calls (e.g., the last 60 seconds).
// minimumNumberOfCalls: The minimum number of calls required in the window before the failure rate or slow call rate can be calculated and the circuit breaker allowed to transition states (e.g., from CLOSED to OPEN). This prevents the circuit from opening prematurely due to a small number of initial failures.
// failureRateThreshold: The percentage of failed calls that will cause the circuit breaker to open.
// waitDurationInOpenState: The amount of time the circuit breaker should remain in the OPEN state before transitioning to HALF_OPEN.

// Timer is

// TODO: on distributed count / counter service?
// CounterStore is the storage abstraction ratelimit uses.
type CounterStore interface {
	// Incr increments a counter at key and returns the new value.
	// TTL tells the store how long to keep the key alive (at least).
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)

	// Get returns the current value of a counter, or 0 if missing.
	Get(ctx context.Context, key string) (int64, error)
}

// Result represents the outcome of a rate limit decision.
type Result struct {
	Allowed       bool
	Remaining     int64         // how many requests left in current window
	RetryAfter    time.Duration // if not allowed, when client may retry
	Limit         int64         // max allowed in window
	Window        time.Duration // configured window size
	WindowResetIn time.Duration // time until current window ends
}

// For application layer rate limiting, key can be userId, remoteIp, etc.
// It is up to the package users to decide on the final string output format.
type Key string

// Limiter decides whether a request associated with `key` is allowed.
type Limiter interface {
	Allow(ctx context.Context, key string) (Result, error)
}
