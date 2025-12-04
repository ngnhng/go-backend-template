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

package serde

// Helper to address the UnaddressableOperand issue.
//
// An expression is addressable if it denotes a specific, stable storage location in memory.
// Value such as a function's result is a rvalue a.k.a temporary value and many programming languages including Go distinguishes between value (rvalue) and variable.
//
// In Go, values (rvalue) may exists only transiently (in registers, on stack or be optimized away) so they are not addressable.
// Allowing rvalue addressability would complicate lifetime semantics and disallow some optimizations.
//
// Ptr returns &v, which basically allocates a cell for the value v (allocate the literal and return its pointer).
func Ptr[T any](v T) *T {
	return &v
}
