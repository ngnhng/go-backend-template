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

package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
)

// CreateProfile creates a new profile.
// Returns 201 with Location header on success, 422 for validation errors, 409 for duplicates.
func (p *ProfileAPI) CreateProfile(ctx context.Context, request api.CreateProfileRequestObject) (api.CreateProfileResponseObject, error) {
	profile, err := p.app.CreateProfile(ctx, request.Body.Name, string(*request.Body.Email))
	if err != nil {
		prob := ProblemFromDomainError(err)
		slog.DebugContext(ctx, "domain error", slog.Any("error", err))
		if errors.Is(err, domain.ErrInvalidData) {
			WithInvalidParam("name", "invalid value")(prob)
			return api.CreateProfile422ApplicationProblemPlusJSONResponse(*prob), nil
		}
		status := 500
		if errors.Is(err, domain.ErrDuplicateProfile) {
			status = 409
		}
		return api.CreateProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: status}, nil
	}

	resp := api.SuccessProfile{
		Data: api.Profile{
			Id:   [16]byte(profile.ID.Bytes()),
			Name: profile.Name,
		},
	}
	return api.CreateProfile201JSONResponse{
		Body:    resp,
		Headers: api.CreateProfile201ResponseHeaders{Location: fmt.Sprintf("/v1/profiles/%s", profile.ID)},
	}, nil
}
