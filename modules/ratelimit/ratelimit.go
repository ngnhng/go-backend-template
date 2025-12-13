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

type (
	LimiterFactory func(limit int64, window time.Duration) RateLimiter

	// RateLimiter enforces time-based rate limits, e.g. "100 requests per 60 seconds".
	// It is not meant for generic "last N events"-style count-based windows.
	RateLimiter interface {
		// Allow determines if the outcome for the provided Key will be allowed or rate-limited.
		Allow(ctx context.Context, key Key) (Result, error)
	}

	// For application layer rate limiting, key can be userId, remoteIp, etc.
	// It is up to the package users to decide on the final string output format.
	Key string

	// Result represents the outcome of a rate limit decision.
	Result struct {
		Allowed       bool
		Remaining     int64         // how many requests left in current window
		RetryAfter    time.Duration // if not allowed, when client may retry
		Limit         int64         // max allowed in window
		Window        time.Duration // configured window size
		WindowResetIn time.Duration // time until current window ends
	}
)
