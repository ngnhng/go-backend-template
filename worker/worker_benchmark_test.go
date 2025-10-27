package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"runtime/pprof"
	"strconv"
	"testing"
)

func Benchmark_BlockingPool_SHA256(b *testing.B) {
	payload := make([]byte, 1024)
	_, _ = rand.Read(payload)

	worker := func(ctx context.Context, p []byte) {
		_ = sha256.Sum256(p)
	}

	poolSizes := []int{3, 7, 10, 15, 20, 50, 80, 150}
	for _, s := range poolSizes {
		// for each pool size, create a sub-benchmark
		b.Run(fmt.Sprintf("pool_size=%d", s), func(b *testing.B) {
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()

			ctx := context.Background()
			jobs := make(chan []byte, 1024)

			// annotate profile data with pool size
			labels := pprof.Labels("pool_size", strconv.Itoa(s))
			// run the measured section with those labels (useful
			// if you take CPU/heap profiles while benchmarking)
			pprof.Do(ctx, labels, func(ctx context.Context) {
				b.ResetTimer()
				go func(n int) {
					for range n {
						jobs <- payload
					}
					close(jobs)
				}(b.N)

				BlockingPool(ctx, s, jobs, worker)
			})
		})
	}
}

func Benchmark_BlockingPool_AllocateAndHash(b *testing.B) {
	sizes := []int{1, 2, 4, 8, 16, 32}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("pool_size=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()
			jobs := make(chan int, 1024)

			w := func(_ context.Context, _ int) {
				buf := make([]byte, 4096) // force alloc
				_, _ = rand.Read(buf)     // “IO-ish”
				_ = sha256.Sum256(buf)    // some CPU
			}

			b.ResetTimer()
			go func(n int) {
				for i := range n {
					jobs <- i
				}
				close(jobs)
			}(b.N)

			BlockingPool(ctx, size, jobs, w)
		})
	}
}

func Benchmark_Direct_SHA256(b *testing.B) {
	payload := make([]byte, 1024)
	_, _ = rand.Read(payload)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()

	worker := func(_ context.Context, p []byte) { _, _ = rand.Read(p); _ = sha256.Sum256(p) }
	ctx := context.Background()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		i++
		worker(ctx, payload)
	}
}
