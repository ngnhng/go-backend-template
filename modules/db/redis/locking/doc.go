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

// Example usage:
//
// 	import (
// 		"context"
// 		"log"
// 		"log/slog"
// 		"os"
// 		"time"

// 		"github.com/redis/rueidis"
// 		"github.com/redis/rueidis/rueidislock"

// 		"your/module/locking"
// 	)

// 	func main() {
// 		// 1) Build a rueidislock.Locker
// 		locker, err := rueidislock.NewLocker(rueidislock.LockerOption{
// 			ClientOption: rueidis.ClientOption{
// 				InitAddress: []string{"redis:6379"},
// 				// + your auth, TLS, etc.
// 			},
// 			KeyMajority:    1,    // if single Redis instance
// 			NoLoopTracking: true, // if all Redis >= 7.0.5
// 		})
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 		defer locker.Close()

// 		// 2) Build the executor
// 		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

// 		exec := locking.NewLockingTaskExecutor(
// 			locker,
// 			locking.WithLogger(logger),
// 			locking.WithNamePrefix("dop:jobs:"),
// 			locking.WithWaitForLock(false),           // "try once" semantics
// 			locking.WithAcquireTimeout(5*time.Second), // used only if waitForLock=true
// 		)

// 		// 3) Define the job
// 		jobCfg := locking.LockConfiguration{
// 			Name:           "profile-cleanup",
// 			LockAtMostFor:  2 * time.Minute, // don’t let job run beyond this
// 			LockAtLeastFor: 30 * time.Second, // don’t run more frequently than this
// 		}

// 		job := func(ctx context.Context) error {
// 			// your job logic here; respect ctx for cancellation
// 			// e.g. cleanup old profiles, rebuild caches, etc.
// 			return nil
// 		}

// 		// 4) Run via your scheduler (simplified example)
// 		ticker := time.NewTicker(5 * time.Minute)
// 		defer ticker.Stop()

// 		for {
// 			select {
// 			case <-ticker.C:
// 				go func() {
// 					if err := exec.Execute(context.Background(), jobCfg, job); err != nil {
// 						if errors.Is(err, locking.ErrLockNotAcquired) {
// 							// another node is executing it; normal situation
// 							return
// 						}
// 						logger.Error("job execution failed", slog.String("job", jobCfg.Name), slog.Any("err", err))
// 					}
// 				}()
// 			}
// 		}
// 	}
