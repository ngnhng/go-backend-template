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

package middleware

import (
	"fmt"
	"net/http"
	"time"

	"app/modules/telemetry"
)

// responseRecorder wraps http.ResponseWriter to capture status code and response size
type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	wroteHeader  bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default if WriteHeader is never called
	}
}

// WriteHeader implements http.ResponseWriter
func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.statusCode = code
		r.wroteHeader = true
		r.ResponseWriter.WriteHeader(code)
	}
}

// Write implements http.ResponseWriter
func (r *responseRecorder) Write(b []byte) (int, error) {
	// If WriteHeader hasn't been called yet, Write will implicitly call it with 200
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

// Telemetry creates a middleware that records metrics for ALL HTTP requests.
// This middleware wraps the ResponseWriter to capture status codes and response sizes
// from any layer (validation middleware, handlers, error handlers, etc.).
//
// Place this as the FIRST middleware in the chain to ensure complete coverage.
func Telemetry(metrics *telemetry.HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip metrics if not configured
			if metrics == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			recorder := newResponseRecorder(w)

			// Process request through the rest of the middleware chain and handler
			next.ServeHTTP(recorder, r)

			// Record metrics after request is complete
			durationMs := float64(time.Since(start).Milliseconds())
			metrics.RecordRequest(
				r.Context(),
				r.Method,
				r.URL.Path,
				fmt.Sprintf("%d", recorder.statusCode),
				durationMs,
				recorder.bytesWritten,
			)
		})
	}
}
