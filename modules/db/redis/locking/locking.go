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
package locking

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/rueidis/rueidislock"
)

// TaskFunc is the task signature executed under the distributed lock.
type TaskFunc func(ctx context.Context) error

// LockConfiguration models ShedLock-style configuration for a task lock.
//
//   - Name          : logical lock name (e.g. "app.profile.cleanup")
//   - LockAtMostFor : maximum time the task is allowed to run;
//     we enforce this as a deadline on the task context.
//   - LockAtLeastFor: minimum time the lock will be held once acquired
//     (even if the task returns early).
type LockConfiguration struct {
	Name           string
	LockAtMostFor  time.Duration
	LockAtLeastFor time.Duration
}

// ErrLockNotAcquired is returned when the executor is configured to "try once"
// and the lock is already held by another node.
var ErrLockNotAcquired = errors.New("locking: lock not acquired")

// ErrInvalidConfiguration is returned when LockConfiguration is invalid.
var ErrInvalidConfiguration = errors.New("locking: invalid lock configuration")

// clock is a pluggable time source for testability.
type clock func() time.Time

func defaultClock() time.Time { return time.Now() }

// LockingTaskExecutor coordinates distributed locks around tasks using
// github.com/redis/rueidis/rueidislock.
//
// It is intended for scheduled jobs / background tasks where you want
// "at most one node executes this job at a time".
type LockingTaskExecutor struct {
	locker rueidislock.Locker
	logger *slog.Logger

	// if true, Execute() will block waiting for the lock (locker.WithContext).
	// if false, Execute() will use TryWithContext once and return ErrLockNotAcquired
	// if the lock is already held.
	waitForLock bool

	// Optional timeout for acquiring the lock when waitForLock = true.
	// If zero, we use the caller's ctx as-is.
	acquireTimeout time.Duration

	// Optional prefix applied to all LockConfiguration.Name values.
	// Final Redis lock key name will be: prefix + cfg.Name.
	namePrefix string

	now clock
}

// Option configures a LockingTaskExecutor.
type Option func(*LockingTaskExecutor)

// WithLogger configures structured logging.
func WithLogger(l *slog.Logger) Option {
	return func(e *LockingTaskExecutor) {
		e.logger = l
	}
}

// WithWaitForLock configures whether Execute() should block until the lock
// is acquired (true) or try once and return ErrLockNotAcquired if it fails (false).
func WithWaitForLock(wait bool) Option {
	return func(e *LockingTaskExecutor) {
		e.waitForLock = wait
	}
}

// WithAcquireTimeout sets a max duration to wait for acquiring the lock
// when waitForLock = true. If zero, no extra timeout is added.
func WithAcquireTimeout(d time.Duration) Option {
	return func(e *LockingTaskExecutor) {
		e.acquireTimeout = d
	}
}

// WithNamePrefix adds a prefix to all lock names (e.g. env / app prefix).
// Example: WithNamePrefix("app:profile:") + Name="cleanup" => "app:profile:cleanup".
func WithNamePrefix(prefix string) Option {
	return func(e *LockingTaskExecutor) {
		e.namePrefix = prefix
	}
}

// WithClock overrides the time source (useful in tests).
func WithClock(fn clock) Option {
	return func(e *LockingTaskExecutor) {
		if fn != nil {
			e.now = fn
		}
	}
}

// NewLockingTaskExecutor constructs a new LockingTaskExecutor from a rueidislock.Locker.
//
// The same Locker can be shared by multiple executors with different prefixes / semantics.
func NewLockingTaskExecutor(locker rueidislock.Locker, opts ...Option) *LockingTaskExecutor {
	e := &LockingTaskExecutor{
		locker:         locker,
		waitForLock:    false, // default: "try once" behavior
		acquireTimeout: 0,
		now:            defaultClock,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	return e
}

// Execute acquires a distributed lock (according to the LockConfiguration)
// and, if acquired, runs the given task under that lock.
//
// Semantics:
//
//   - If cfg.LockAtLeastFor > 0:
//   - The lock is held for at least that duration starting from
//     when the task begins execution, even if the task returns early.
//   - If cfg.LockAtMostFor > 0:
//   - The task gets a context with that deadline; if exceeded,
//     ctx.Err() will be context.DeadlineExceeded.
//   - If waitForLock == false:
//   - A single TryWithContext() is performed; if lock is held elsewhere,
//     ErrLockNotAcquired is returned.
//   - If waitForLock == true:
//   - WithContext() is used, optionally bounded by acquireTimeout.
//
// The lock is always released by calling the cancel func returned
// from rueidislock, even if the task panics or returns an error.
func (e *LockingTaskExecutor) Execute(
	ctx context.Context,
	cfg LockConfiguration,
	task TaskFunc,
) error {
	if task == nil {
		return errors.New("locking: task must not be nil")
	}

	if err := validateConfig(cfg); err != nil {
		return err
	}

	lockName := e.lockName(cfg.Name)

	if e.logger != nil {
		e.logger.Info("locking: attempting to acquire lock",
			slog.String("lock.name", lockName),
			slog.Duration("lock.at_most_for", cfg.LockAtMostFor),
			slog.Duration("lock.at_least_for", cfg.LockAtLeastFor),
			slog.Bool("lock.wait_for_lock", e.waitForLock),
		)
	}

	// 1) Acquire the lock (blocking or try-once).
	acquiredAt := e.now()

	var (
		lockCtx    context.Context
		lockCancel context.CancelFunc
		err        error
	)

	if e.waitForLock {
		// Blocking mode: WithContext
		acquireCtx := ctx
		if e.acquireTimeout > 0 {
			var cancel context.CancelFunc
			acquireCtx, cancel = context.WithTimeout(ctx, e.acquireTimeout)
			defer cancel()
		}

		lockCtx, lockCancel, err = e.locker.WithContext(acquireCtx, lockName)
		if err != nil {
			// ErrLockerClosed means the locker client is unusable now.
			if errors.Is(err, rueidislock.ErrLockerClosed) {
				return fmt.Errorf("locking: locker closed while acquiring lock %q: %w", lockName, err)
			}
			// Context errors should be surfaced as-is.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return err
			}
			return fmt.Errorf("locking: failed to acquire lock %q: %w", lockName, err)
		}
	} else {
		// Try-once mode: TryWithContext
		lockCtx, lockCancel, err = e.locker.TryWithContext(ctx, lockName)
		if err != nil {
			if errors.Is(err, rueidislock.ErrNotLocked) {
				// Someone else already holds the lock.
				if e.logger != nil {
					e.logger.Info("locking: lock not acquired (already held by another node)",
						slog.String("lock.name", lockName))
				}
				return ErrLockNotAcquired
			}
			if errors.Is(err, rueidislock.ErrLockerClosed) {
				return fmt.Errorf("locking: locker closed while trying to acquire lock %q: %w", lockName, err)
			}
			return fmt.Errorf("locking: failed to try-acquire lock %q: %w", lockName, err)
		}
	}

	defer func() {
		// Release the underlying lock.
		lockCancel()
	}()

	if e.logger != nil {
		e.logger.Info("locking: lock acquired",
			slog.String("lock.name", lockName),
			slog.Duration("lock.acquire_latency", e.now().Sub(acquiredAt)),
		)
	}

	// 2) Build the task context bounded by LockAtMostFor.
	taskCtx := lockCtx
	var taskCancel context.CancelFunc

	if cfg.LockAtMostFor > 0 {
		taskCtx, taskCancel = context.WithTimeout(lockCtx, cfg.LockAtMostFor)
	} else {
		taskCtx, taskCancel = context.WithCancel(lockCtx)
	}
	defer taskCancel()

	// 3) Run the task and measure its execution time.
	taskStart := e.now()
	err = task(taskCtx)
	taskEnd := e.now()
	taskDuration := taskEnd.Sub(taskStart)

	if e.logger != nil {
		e.logger.Info("locking: task finished",
			slog.String("lock.name", lockName),
			slog.Duration("task.duration", taskDuration),
			slog.Any("task.error", err),
		)
	}

	// 4) Enforce LockAtLeastFor: keep the lock for at least that duration
	//    starting from when the task began, unless the outer ctx / lockCtx
	//    has already been canceled.
	if cfg.LockAtLeastFor > 0 {
		minHoldUntil := taskStart.Add(cfg.LockAtLeastFor)
		now := e.now()

		if now.Before(minHoldUntil) {
			wait := minHoldUntil.Sub(now)

			if e.logger != nil {
				e.logger.Info("locking: enforcing lockAtLeastFor",
					slog.String("lock.name", lockName),
					slog.Duration("lock.remaining_hold", wait),
				)
			}

			timer := time.NewTimer(wait)
			defer timer.Stop()

			select {
			case <-timer.C:
				// normal completion, we kept the lock long enough
			case <-ctx.Done():
				// caller canceled; respect it
			case <-lockCtx.Done():
				// lock lost externally (e.g. redis issues or key deleted)
			}
		}
	}

	// After this function returns, defer lockCancel() runs and releases the lock.
	return err
}

func (e *LockingTaskExecutor) lockName(base string) string {
	if e.namePrefix == "" {
		return base
	}
	return e.namePrefix + base
}

func validateConfig(cfg LockConfiguration) error {
	if cfg.Name == "" {
		return fmt.Errorf("%w: lock name must not be empty", ErrInvalidConfiguration)
	}
	if cfg.LockAtMostFor < 0 {
		return fmt.Errorf("%w: lockAtMostFor must not be negative", ErrInvalidConfiguration)
	}
	if cfg.LockAtLeastFor < 0 {
		return fmt.Errorf("%w: lockAtLeastFor must not be negative", ErrInvalidConfiguration)
	}
	if cfg.LockAtMostFor > 0 && cfg.LockAtLeastFor > cfg.LockAtMostFor {
		return fmt.Errorf("%w: lockAtLeastFor (%s) > lockAtMostFor (%s)",
			ErrInvalidConfiguration, cfg.LockAtLeastFor, cfg.LockAtMostFor)
	}
	return nil
}
