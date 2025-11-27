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
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// ValidationError represents a structured validation error with field and reason.
type ValidationError struct {
	Field  string
	Reason string
}

// ExtractValidationErrors extracts structured validation errors from an OpenAPI validation error.
func ExtractValidationErrors(err error) []ValidationError {
	var errors []ValidationError

	switch v := err.(type) {
	case openapi3.MultiError:
		for _, item := range v {
			errors = append(errors, ExtractValidationErrors(item)...)
		}
	default:
		errors = append(errors, extractSingleError(v))
	}

	return errors
}

func extractSingleError(err error) ValidationError {
	// Handle RequestError to extract param or body pointer
	if re, ok := err.(*openapi3filter.RequestError); ok {
		// SchemaError provides pointer and reason
		if se, ok := re.Err.(*openapi3.SchemaError); ok {
			// Build a field name from JSON pointer
			ptr := "/" + strings.Join(se.JSONPointer(), "/")
			if re.Parameter != nil {
				return ValidationError{Field: re.Parameter.Name, Reason: se.Reason}
			}
			field := extractFieldFromPointer(ptr)
			return ValidationError{Field: field, Reason: se.Reason}
		}
		// Fallback when not a SchemaError: do not echo input; keep messages generic
		if re.Parameter != nil {
			return ValidationError{Field: re.Parameter.Name, Reason: SafeReason(re.Reason)}
		}
		return ValidationError{Field: "body", Reason: SafeReason(re.Reason)}
	}

	// Direct SchemaError (commonly within MultiError)
	if se, ok := err.(*openapi3.SchemaError); ok {
		ptr := "/" + strings.Join(se.JSONPointer(), "/")
		field := extractFieldFromPointer(ptr)
		return ValidationError{Field: field, Reason: se.Reason}
	}

	// Security error
	if _, ok := err.(*openapi3filter.SecurityRequirementsError); ok {
		return ValidationError{Field: "authorization", Reason: "missing or invalid credentials"}
	}

	// Generic fallback
	return ValidationError{Field: "request", Reason: "invalid value"}
}

func extractFieldFromPointer(ptr string) string {
	field := strings.TrimPrefix(ptr, "/")
	if idx := strings.Index(field, "/"); idx >= 0 {
		field = field[:idx]
	}
	if field == "" || field == "0" {
		field = "body"
	}
	return field
}

// InferBodyValidationStatus returns 422 for body/schema violations to avoid 400 on well-formed but semantically invalid payloads.
func InferBodyValidationStatus(err error) int {
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
			if InferBodyValidationStatus(item) == http.StatusUnprocessableEntity {
				return http.StatusUnprocessableEntity
			}
		}
	case *openapi3.SchemaError:
		return http.StatusUnprocessableEntity
	}
	return 0
}

// SafeReason reduces verbose reasons to avoid reflecting input data back to the client.
func SafeReason(reason string) string {
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
