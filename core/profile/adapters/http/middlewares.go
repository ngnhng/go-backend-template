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
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

// RecoverHTTPMiddleware recovers from panics at the outermost net/http layer.
func RecoverHTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("panic", slog.Any("error", rec))
					writeProblem(w, InternalProblem("server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Cached OpenAPI spec for validator
var (
	specOnce sync.Once
	specDoc  *openapi3.T
	specErr  error
)

func loadProfileSpecOnce() (*openapi3.T, error) {
	// TODO: embed.FS
	specOnce.Do(func() {
		path := filepath.FromSlash("oapi/profile-api-spec.yaml")
		loader := openapi3.NewLoader()
		loader.IsExternalRefsAllowed = true
		specDoc, specErr = loader.LoadFromFile(path)
	})
	return specDoc, specErr
}

// ProfileHTTPValidationMiddleware validates requests using nethttp-middleware before body decoding.
func ProfileHTTPValidationMiddleware() func(http.Handler) http.Handler {
	spec, err := loadProfileSpecOnce()
	if err != nil {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeProblem(w, InternalProblem("server error"))
			})
		}
	}

	opts := &nethttpmiddleware.Options{
		Options:               openapi3filter.Options{MultiError: true},
		DoNotValidateServers:  true,
		SilenceServersWarning: true,
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, eopts nethttpmiddleware.ErrorHandlerOpts) {
			status := eopts.StatusCode
			if status == 0 {
				status = http.StatusBadRequest
			}
			// Body schema violations should be 422
			if hint := inferBodyValidationStatus(err); hint == http.StatusUnprocessableEntity {
				status = http.StatusUnprocessableEntity
			}

			problem := NewErrorResponse(WithTitle(http.StatusText(status)), WithStatus(status), WithDetail("validation failed"))

			// Expand errors into invalidParams with pointers where possible
			switch v := err.(type) {
			case openapi3.MultiError:
				for _, item := range v {
					addValidationDetail(problem, item)
				}
			default:
				addValidationDetail(problem, v)
			}

			writeProblem(w, problem)
		},
	}

	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, opts)
}

func addValidationDetail(problem *ErrorResponse, err error) {
	// Handle RequestError to extract param or body pointer
	if re, ok := err.(*openapi3filter.RequestError); ok {
		// SchemaError provides pointer and reason
		if se, ok := re.Err.(*openapi3.SchemaError); ok {
			// Build a field name from JSON pointer
			ptr := "/" + strings.Join(se.JSONPointer(), "/")
			if re.Parameter != nil {
				WithInvalidParam(re.Parameter.Name, se.Reason)(problem)
				return
			}
			field := strings.TrimPrefix(ptr, "/")
			if idx := strings.Index(field, "/"); idx >= 0 {
				field = field[:idx]
			}
			if field == "" || field == "0" {
				field = "body"
			}
			WithInvalidParam(field, se.Reason)(problem)
			return
		}
		// Fallback when not a SchemaError: do not echo input; keep messages generic
		if re.Parameter != nil {
			WithInvalidParam(re.Parameter.Name, safeReason(re.Reason))(problem)
		} else {
			WithInvalidParam("body", safeReason(re.Reason))(problem)
		}
		return
	}
	// Direct SchemaError (commonly within MultiError)
	if se, ok := err.(*openapi3.SchemaError); ok {
		ptr := "/" + strings.Join(se.JSONPointer(), "/")
		field := strings.TrimPrefix(ptr, "/")
		if idx := strings.Index(field, "/"); idx >= 0 {
			field = field[:idx]
		}
		if field == "" || field == "0" {
			field = "body"
		}
		WithInvalidParam(field, se.Reason)(problem)
		return
	}
	// Security error
	if _, ok := err.(*openapi3filter.SecurityRequirementsError); ok {
		WithInvalidParam("authorization", "missing or invalid credentials")(problem)
		return
	}
	// Generic fallback
	WithInvalidParam("request", "invalid value")(problem)
}

// inferBodyValidationStatus returns 422 for body/schema violations to avoid 400 on well-formed but semantically invalid payloads.
func inferBodyValidationStatus(err error) int {
	switch v := err.(type) {
	case *openapi3filter.RequestError:
		if v.RequestBody != nil {
			return http.StatusUnprocessableEntity
		}
		if _, ok := v.Err.(*openapi3.SchemaError); ok {
			return http.StatusUnprocessableEntity
		}
	case openapi3.MultiError:
		for _, item := range v {
			if inferBodyValidationStatus(item) == http.StatusUnprocessableEntity {
				return http.StatusUnprocessableEntity
			}
		}
	case *openapi3.SchemaError:
		return http.StatusUnprocessableEntity
	}
	return 0
}

// safeReason reduces verbose reasons to avoid reflecting input data back to the client.
func safeReason(reason string) string {
	if reason == "" {
		return "invalid value"
	}
	// Strip common noisy fragments
	lower := strings.ToLower(reason)
	if strings.Contains(lower, "doesn't match schema") {
		return "doesn't match schema"
	}
	if strings.Contains(lower, "must be one of") {
		return reason
	}
	// Default generic message
	return "invalid value"
}
