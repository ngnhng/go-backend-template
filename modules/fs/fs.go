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

package fs

import "io/fs"

type FS interface {
	// Open opens the named file for reading
	Open(name string) (fs.File, error)

	// ReadDir reads the contents of the directory and returns a slice of directory entries
	ReadDir(name string) ([]fs.DirEntry, error)

	// ReadFile reads the named file and returns its contents
	ReadFile(name string) ([]byte, error)
}

type LocalFS struct{}
