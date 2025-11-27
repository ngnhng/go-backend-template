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

package http

import (
	"context"
	"net/http"

	"app/middleware"
)

// RecoverHTTPMiddleware returns a panic recovery middleware configured for the Profile API.
func RecoverHTTPMiddleware() func(http.Handler) http.Handler {
	return middleware.Recovery(func(w http.ResponseWriter, r *http.Request, recovered any) {
		WriteProblem(w, InternalProblem("server error"))
	})
}

// ProfileHTTPValidationMiddleware returns an OpenAPI validation middleware for the Profile API.
func ProfileHTTPValidationMiddleware() func(http.Handler) http.Handler {
	return middleware.OpenAPIValidation(
		"oapi/profile-api-spec.yaml",
		// Validation error handler
		func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, statusCode int) {
			problem := NewErrorResponse(
				WithTitle(http.StatusText(statusCode)),
				WithStatus(statusCode),
				WithDetail("validation failed"),
			)

			// Extract validation errors and add to problem details
			validationErrors := middleware.ExtractValidationErrors(err)
			for _, ve := range validationErrors {
				WithInvalidParam(ve.Field, ve.Reason)(problem)
			}

			WriteProblem(w, problem)
		},
		// Spec load error handler
		func(w http.ResponseWriter, r *http.Request, err error) {
			WriteProblem(w, InternalProblem("server error"))
		},
	)
}
