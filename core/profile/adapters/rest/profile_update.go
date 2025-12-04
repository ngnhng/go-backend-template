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
	"strconv"

	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
	"app/modules/etag"

	"github.com/gofrs/uuid/v5"
)

// UpdateProfile performs a full replacement of a profile (PUT semantics).
// Requires If-Match header with current ETag for optimistic concurrency control.
// Returns 200 with new ETag on success, 412 if version mismatch, 404 if not found.
func (p *ProfileAPI) UpdateProfile(ctx context.Context, request api.UpdateProfileRequestObject) (api.UpdateProfileResponseObject, error) {
	uid, err := uuid.FromBytes(request.Id[:])
	if err != nil {
		prob := BadRequestProblem("invalid id")
		WithInvalidParam("id", "invalid value")(prob)
		return api.UpdateProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	// Parse version from ETag without querying database
	versionStr, err := etag.ParseETag(string(request.Params.IfMatch))
	if err != nil {
		prob := BadRequestProblem("invalid etag format")
		WithInvalidParam("If-Match", "invalid etag format")(prob)
		return api.UpdateProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		prob := BadRequestProblem("invalid etag version")
		WithInvalidParam("If-Match", "invalid version in etag")(prob)
		return api.UpdateProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	// If email is not provided, fetch current profile to preserve existing email
	// (PUT semantics with optional field for backwards compatibility)
	var emailVal string
	if request.Body != nil && request.Body.Email != nil {
		emailVal = string(*request.Body.Email)
	} else {
		current, err := p.app.GetProfileByID(ctx, uid)
		if err != nil {
			prob := ProblemFromDomainError(err)
			switch {
			case errors.Is(err, domain.ErrInvalidData):
				WithInvalidParam("id", "invalid value")(prob)
				return api.UpdateProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
			case errors.Is(err, domain.ErrProfileNotFound):
				return api.UpdateProfile404ApplicationProblemPlusJSONResponse(*prob), nil
			default:
				return api.UpdateProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 500}, nil
			}
		}
		emailVal = current.Email
	}

	params := &domain.UpdateProfileParams{
		ID:      uid,
		Name:    request.Body.Name,
		Email:   emailVal,
		Version: version,
	}
	updated, err := p.app.UpdateProfile(ctx, params)
	if err != nil {
		prob := ProblemFromDomainError(err)
		switch {
		case errors.Is(err, domain.ErrInvalidData):
			WithInvalidParam("name", "invalid value")(prob)
			return api.UpdateProfile422ApplicationProblemPlusJSONResponse(*prob), nil
		case errors.Is(err, domain.ErrDuplicateProfile):
			return api.UpdateProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 409}, nil
		case errors.Is(err, domain.ErrProfileNotFound):
			return api.UpdateProfile404ApplicationProblemPlusJSONResponse(*prob), nil
		case errors.Is(err, domain.ErrPrecondition):
			// On version mismatch, fetch latest to return current ETag in response
			latest, fetchErr := p.app.GetProfileByID(ctx, uid)
			etagVal := string(request.Params.IfMatch)
			if fetchErr == nil {
				etagVal = etag.ETag(latest)
			}
			return api.UpdateProfile412ApplicationProblemPlusJSONResponse{
				PreconditionFailedResponseApplicationProblemPlusJSONResponse: api.PreconditionFailedResponseApplicationProblemPlusJSONResponse{
					Body:    *prob,
					Headers: api.PreconditionFailedResponseResponseHeaders{ETag: etagVal},
				},
			}, nil
		default:
			return api.UpdateProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 500}, nil
		}
	}
	resp := api.SuccessProfile{Data: mapProfile([]domain.Profile{*updated})[0]}
	return api.UpdateProfile200JSONResponse{
		Body: resp,
		Headers: api.UpdateProfile200ResponseHeaders{
			ETag: etag.ETag(updated),
		},
	}, nil
}
