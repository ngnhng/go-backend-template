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

package services

import (
	"net/http"

	profile_api "app/api/profileapi/stdlib"
	profile_http "app/core/profile/adapters/http"
)

// ProfileAPIService encapsulates the registration logic for the Profile API.
type ProfileAPIService struct {
	handler profile_api.StrictServerInterface
}

func NewProfileAPIService(h profile_api.StrictServerInterface) *ProfileAPIService {
	return &ProfileAPIService{handler: h}
}

// Register configures the strict handler and mounts the profile API routes.
func (s *ProfileAPIService) Register(mux *http.ServeMux) {
	strict := profile_api.NewStrictHandlerWithOptions(
		s.handler,
		[]profile_api.StrictMiddlewareFunc{},
		profile_api.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  profile_http.ProblemDetailsRequestErrorHandler,
			ResponseErrorHandlerFunc: profile_http.ProblemDetailsResponseErrorHandler,
		},
	)

	profile_api.HandlerWithOptions(
		strict,
		profile_api.StdHTTPServerOptions{
			BaseRouter:       mux,
			Middlewares:      []profile_api.MiddlewareFunc{},
			ErrorHandlerFunc: profile_http.ProblemDetailsRequestErrorHandler,
		},
	)
}

// Middlewares returns global middlewares required by the Profile API, such as validation.
func (s *ProfileAPIService) Middlewares() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		profile_http.ProfileHTTPValidationMiddleware(),
	}
}
