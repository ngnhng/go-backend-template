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
	profileapi "app/api/profileapi/stdlib"
	profile_service "app/profile-service"
	"net/http"
)

// ProfileAPIService encapsulates the registration logic for the Profile API.
type ProfileAPIService struct {
	handler profileapi.StrictServerInterface
}

func NewProfileAPIService(h profileapi.StrictServerInterface) *ProfileAPIService {
	return &ProfileAPIService{handler: h}
}

// Register configures the strict handler and mounts the profile API routes.
func (s *ProfileAPIService) Register(mux *http.ServeMux) {
	strict := profileapi.NewStrictHandlerWithOptions(
		s.handler,
		[]profileapi.StrictMiddlewareFunc{},
		profileapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  profile_service.ProblemDetailsRequestErrorHandler,
			ResponseErrorHandlerFunc: profile_service.ProblemDetailsResponseErrorHandler,
		},
	)

	profileapi.HandlerWithOptions(
		strict,
		profileapi.StdHTTPServerOptions{
			BaseRouter:       mux,
			Middlewares:      []profileapi.MiddlewareFunc{},
			ErrorHandlerFunc: profile_service.ProblemDetailsRequestErrorHandler,
		},
	)
}

// Middlewares returns global middlewares required by the Profile API, such as validation.
func (s *ProfileAPIService) Middlewares() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		profile_service.ProfileHTTPValidationMiddleware(),
	}
}
