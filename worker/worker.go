// Copyright 2025 Nguyen Nhat Nguyen
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

package worker

import (
	"context"
	"sync"
)

type Worker[Job any] func(context.Context, Job)

// BlockingPool spawns worker to execute the jobs.
//
// The function will block until all the work are done (no more jobs e.g. channel closed)
//
// # The caller must ensure that jobs channel eventualy gets closed or the context gets cancelled.
//
// Use a goroutine worker pool when:
//
// The workload is unbounded or high volume. A pool prevents uncontrolled goroutine growth,
// which can lead to memory exhaustion, GC pressure, and unpredictable performance.
//
// Unbounded concurrency risks resource saturation. Capping the number of concurrent
// workers helps avoid overwhelming the CPU, network, database, or disk I/O—especially
// under load.
//
// You need predictable parallelism for stability. Limiting concurrency smooths out
// performance spikes and keeps system behavior consistent, even during traffic surges.
//
// Tasks are relatively uniform and queue-friendly. When task cost is consistent, a fixed
// pool size provides efficient scheduling with minimal overhead, ensuring good throughput
// without complex coordination.
//
// Avoid a worker pool when:
//
// Each task must be processed immediately with minimal latency. Queuing in a worker pool
// introduces delay. For latency-critical tasks, direct goroutine spawning avoids the
// scheduling overhead.
//
// You can rely on Go's scheduler for natural load balancing in low-load scenarios. In
// light workloads, the overhead of managing a pool may outweigh its benefits. Go’s
// scheduler can often handle lightweight parallelism efficiently on its own.
//
// Workload volume is small and bounded. Spinning up goroutines directly keeps code
// simpler for limited, predictable workloads without risking uncontrolled growth.
func BlockingPool[Job any](ctx context.Context, size int, jobs <-chan Job, worker Worker[Job]) {
	if size <= 0 {
		size = 1
	}
	wg := sync.WaitGroup{}
	// spawn workers that pull jobs while listening for channel closure and ctx cancellation
	for range size {
		wg.Go(func() {
			// wg.Go requires that func does not panic
			defer func() { _ = recover() }()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					worker(ctx, job)
				}
			}
		})
	}

	wg.Wait()
}
