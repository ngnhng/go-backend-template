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

package middleware

import (
	"log/slog"
	"net/http"
)

// PanicHandler is a function that handles panics and writes an appropriate response.
type PanicHandler func(w http.ResponseWriter, r *http.Request, recovered any)

// Recovery creates a middleware that recovers from panics and calls the provided handler.
func Recovery(handler PanicHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("panic", slog.Any("error", rec))
					handler(w, r, rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
