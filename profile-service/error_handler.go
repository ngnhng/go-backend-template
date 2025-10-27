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

package profile_service

import (
	api "app/api/profileapi"
	"app/api/serde"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

type (
	ErrorResponse       = api.Problem
	ErrorResponseOption func(*ErrorResponse)
)

func writeProblem(w http.ResponseWriter, p *ErrorResponse) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func WithHTTPCode(code int) func(*ErrorResponse) {
	if code < 200 || code > 599 {
		code = 599
	}
	return func(er *ErrorResponse) {
		er.Code = serde.Ptr(strconv.Itoa(code))
	}
}

func WithDetail(message string) func(*ErrorResponse) {
	return func(er *ErrorResponse) {
		er.Detail = &message
	}
}

func WithTitle(title string) func(*ErrorResponse) {
	return func(er *ErrorResponse) {
		er.Title = title
	}
}

func WithStatus(status int) func(*ErrorResponse) {
	return func(er *ErrorResponse) {
		er.Status = status
	}
}

func WithInvalidParam(name, reason string) func(*ErrorResponse) {
	return func(er *ErrorResponse) {
		if er.InvalidParams == nil {
			s := []struct {
				Name   string `json:"name"`
				Reason string `json:"reason"`
			}{{Name: name, Reason: reason}}
			er.InvalidParams = &s
			return
		}
		s := append(*er.InvalidParams, struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}{Name: name, Reason: reason})
		er.InvalidParams = &s
	}
}

func NewErrorResponse(opts ...ErrorResponseOption) *ErrorResponse {
	e := &ErrorResponse{
		Type:   serde.Ptr("about:blank"),
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: serde.Ptr("unhandled error"),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Convenience builders for common problem types
func BadRequestProblem(detail string, opts ...ErrorResponseOption) *ErrorResponse {
	base := []ErrorResponseOption{WithTitle("Bad Request"), WithStatus(http.StatusBadRequest), WithDetail(detail)}
	return NewErrorResponse(append(base, opts...)...)
}

func ValidationProblem(detail string, opts ...ErrorResponseOption) *ErrorResponse {
	base := []ErrorResponseOption{WithTitle("Unprocessable Entity"), WithStatus(http.StatusUnprocessableEntity), WithDetail(detail)}
	return NewErrorResponse(append(base, opts...)...)
}

func ConflictProblem(detail string, opts ...ErrorResponseOption) *ErrorResponse {
	base := []ErrorResponseOption{WithTitle("Conflict"), WithStatus(http.StatusConflict), WithDetail(detail)}
	return NewErrorResponse(append(base, opts...)...)
}

func InternalProblem(detail string) *ErrorResponse {
	return NewErrorResponse(WithTitle("Internal Server Error"), WithStatus(http.StatusInternalServerError), WithDetail(detail))
}

// ProblemFromDomainError maps domain/service-layer sentinel errors to RFC7807 problems.
func ProblemFromDomainError(err error) *ErrorResponse {
	switch {
	case errors.Is(err, ErrDuplicateProfile):
		return ConflictProblem("profile with this name already exists")
	case errors.Is(err, ErrInvalidData):
		return ValidationProblem("validation failed")
	default:
		return InternalProblem("server error")
	}
}

// ProblemDetailsResponseErrorHandler centralizes unexpected handler errors
// occurring after request parsing, emitting a generic 500 Problem.
func ProblemDetailsResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// Generic 500 Problem for unexpected handler errors.
	_ = err // avoid leaking internal error details to clients
	writeProblem(w, InternalProblem("server error"))
}

func ProblemDetailsRequestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// Default to RFC7807 Bad Request for request decoding/binding issues
	problem := NewErrorResponse(
		WithTitle("Bad Request"),
		WithStatus(http.StatusBadRequest),
		WithDetail("invalid request parameter(s)"),
	)

	switch e := err.(type) {
	case *api.InvalidParamFormatError:
		WithInvalidParam(e.ParamName, e.Err.Error())(problem)
	case *api.RequiredParamError:
		WithInvalidParam(e.ParamName, "parameter is required")(problem)
	case *api.RequiredHeaderError:
		WithInvalidParam(e.ParamName, "header is required")(problem)
	case *api.UnmarshalingParamError:
		WithInvalidParam(e.ParamName, e.Err.Error())(problem)
	case *api.TooManyValuesForParamError:
		WithInvalidParam(e.ParamName, fmt.Sprintf("expected one value, got %d", e.Count))(problem)
	case *api.UnescapedCookieParamError:
		WithInvalidParam(e.ParamName, e.Err.Error())(problem)
	default:
		// Fallback: include error message as detail to aid clients
		if err != nil {
			WithDetail(err.Error())(problem)
		}
	}

	writeProblem(w, problem)
}
