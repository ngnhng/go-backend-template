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
	"errors"

	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
	"app/modules/etag"

	"github.com/gofrs/uuid/v5"
)

// GetProfileById retrieves a single profile by its UUID.
// Returns 200 with ETag header on success, 404 if not found.
func (p *ProfileAPI) GetProfileById(ctx context.Context, request api.GetProfileByIdRequestObject) (api.GetProfileByIdResponseObject, error) {
	uid, err := uuid.FromBytes(request.Id[:])
	if err != nil {
		prob := BadRequestProblem("invalid id")
		WithInvalidParam("id", "invalid value")(prob)
		return api.GetProfileById400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}
	prof, err := p.app.GetProfileByID(ctx, uid)
	if err != nil {
		prob := ProblemFromDomainError(err)
		switch {
		case errors.Is(err, domain.ErrInvalidData):
			WithInvalidParam("id", "invalid value")(prob)
			return api.GetProfileById400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
		case errors.Is(err, domain.ErrProfileNotFound):
			return api.GetProfileById404ApplicationProblemPlusJSONResponse(*prob), nil
		default:
			return api.GetProfileByIddefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 500}, nil
		}
	}
	resp := api.SuccessProfile{Data: mapProfile([]domain.Profile{*prof})[0]}
	return api.GetProfileById200JSONResponse{
		Body: resp,
		Headers: api.GetProfileById200ResponseHeaders{
			ETag: etag.ETag(prof),
		},
	}, nil
}
