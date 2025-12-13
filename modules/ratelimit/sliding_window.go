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
	"fmt"
	"math/bits"
	"time"

	"app/modules/clock"
)

var _ RateLimiter = (*SlidingWindowRateLimiter)(nil)

// Time-based 2-window sliding counter implementation of the rate limiter.
// It uses a time-based sliding window implemented as two adjacent fixed windows (current + previous) and interpolates between them.
type (
	SlidingWindowRateLimiter struct {
		clock     clock.Clock
		counter   CounterStore
		keyPrefix string

		limit  uint64
		window time.Duration
	}
)

func SlidingWindowFactory(clock clock.Clock, counter CounterStore, keyPrefix string) LimiterFactory {
	return func(l int64, w time.Duration) RateLimiter {
		return &SlidingWindowRateLimiter{
			clock,
			counter,
			keyPrefix,
			uint64(l),
			w,
		}
	}
}

// Allow implements RateLimiter.
func (s *SlidingWindowRateLimiter) Allow(ctx context.Context, key Key) (Result, error) {
	now := s.clock.Now()
	nowNs := now.UnixNano()
	windowNs := s.window.Nanoseconds()
	// the current window we are in
	currentWindowIdx := nowNs / windowNs
	currentWindowCount, err := s.incrementWindow(ctx, key, currentWindowIdx)
	if err != nil {
		return Result{}, err
	}

	currentWindowStartNs := currentWindowIdx * windowNs

	prevKey := s.buildKey(key, currentWindowIdx-1)

	prevWindowCount, err := s.counter.Get(ctx, prevKey)
	if err != nil {
		return Result{}, err
	}

	currentWindowCount = max(currentWindowCount, 0)
	prevWindowCount = max(prevWindowCount, 0)

	currentWindowElapsedNs := nowNs - currentWindowStartNs
	currentWindowElapsedNs = min(currentWindowElapsedNs, windowNs)
	currentWindowElapsedNs = max(currentWindowElapsedNs, 0)
	prevWindowWeightNs := windowNs - currentWindowElapsedNs

	windowResetIn := max(s.window-time.Duration(currentWindowElapsedNs), 0)

	windowNsU := uint64(windowNs)

	// maintain accuracy by avoiding float64
	// preventing cases like similar remaining for two consecutive requests
	// we do:
	// usage = current_count * window + prev_count * prev_weight)
	// then compare usage to limit*window (same unit)
	curHi, curLo := bits.Mul64(uint64(currentWindowCount), windowNsU)
	prevHi, prevLo := bits.Mul64(uint64(prevWindowCount), uint64(prevWindowWeightNs))
	usageLo, carry := bits.Add64(curLo, prevLo, 0)
	usageHi, _ := bits.Add64(curHi, prevHi, carry)

	limitHi, limitLo := bits.Mul64(s.limit, windowNsU)
	allowed := usageHi < limitHi || (usageHi == limitHi && usageLo <= limitLo)

	// Assume used request is max uint64 if we later cannot calculate the correct value
	usedRequestsCeil := uint64(^uint64(0))

	// try to find the usage_requests = ceil(usage / window)
	if usageHi == 0 {
		// plain integer division truncates (usage / window = floor(usage / window))
		// so we add (divisor - 1 ) to round up when there is remainder
		usedRequestsCeil = (usageLo + windowNsU - 1) / windowNsU
	} else if usageHi < windowNsU {
		q, r := bits.Div64(usageHi, usageLo, windowNsU)
		usedRequestsCeil = q
		// add one for remainder
		// and also check against maxUint64 to prevent overflow
		if r != 0 && usedRequestsCeil != ^uint64(0) {
			usedRequestsCeil++
		}
	}

	remainingU := uint64(0)
	if usedRequestsCeil < s.limit {
		remainingU = s.limit - usedRequestsCeil
	}

	result := Result{
		Allowed:       allowed,
		Remaining:     int64(remainingU),
		RetryAfter:    windowResetIn,
		Limit:         int64(s.limit),
		Window:        s.window,
		WindowResetIn: windowResetIn,
	}

	if result.Allowed {
		result.RetryAfter = 0
	}

	return result, nil
}

func (s *SlidingWindowRateLimiter) buildKey(key Key, windowIdx int64) string {
	return fmt.Sprintf("%s:%s:%d", s.keyPrefix, key, windowIdx)
}

func (s *SlidingWindowRateLimiter) incrementWindow(ctx context.Context, key Key, windowIdx int64) (int64, error) {
	k := s.buildKey(key, windowIdx)
	return s.counter.Incr(ctx, k, s.window*2)
}
