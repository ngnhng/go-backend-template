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
	"crypto/sha256"
	"fmt"
	"testing"
)

func Test_100_Workload(t *testing.T) {

	jobs := make(chan int)
	worker := func(ctx context.Context, n int) {
		sha256.Sum256(fmt.Appendf([]byte{}, "payload %d", n))
	}

	go func() {
		defer close(jobs)
		for i := range 100 {
			jobs <- i
		}
	}()

	BlockingPool(context.Background(), 5, jobs, worker)
}

func Test_10K_Workload(t *testing.T) {

}
