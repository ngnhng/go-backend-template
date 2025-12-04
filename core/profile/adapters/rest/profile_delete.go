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
	"net/http"
	"strconv"

	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
	"app/modules/etag"

	"github.com/gofrs/uuid/v5"
)

// DeleteProfile soft-deletes a profile.
// Requires If-Match header with current ETag for optimistic concurrency control.
// Returns 204 on success, 412 if version mismatch, 404 if not found.
func (p *ProfileAPI) DeleteProfile(ctx context.Context, request api.DeleteProfileRequestObject) (api.DeleteProfileResponseObject, error) {
	uid, err := uuid.FromBytes(request.Id[:])
	if err != nil {
		prob := BadRequestProblem("invalid id")
		WithInvalidParam("id", "invalid value")(prob)
		return api.DeleteProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	ifMatch := request.Params.IfMatch
	if ifMatch == "" {
		prob := BadRequestProblem("missing if-match header")
		WithInvalidParam("If-Match", "header is required")(prob)
		return api.DeleteProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	// Parse version from ETag without querying database
	versionStr, err := etag.ParseETag(ifMatch)
	if err != nil {
		prob := BadRequestProblem("invalid etag format")
		WithInvalidParam("If-Match", "invalid etag format")(prob)
		return api.DeleteProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		prob := BadRequestProblem("invalid etag version")
		WithInvalidParam("If-Match", "invalid version in etag")(prob)
		return api.DeleteProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	if err := p.app.DeleteProfile(ctx, uid, version); err != nil {
		prob := ProblemFromDomainError(err)
		switch {
		case errors.Is(err, domain.ErrInvalidData):
			WithInvalidParam("id", "invalid value")(prob)
			return api.DeleteProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
		case errors.Is(err, domain.ErrPrecondition):
			prob = PreconditionProblem("etag mismatch")
			return api.DeleteProfiledefaultApplicationProblemPlusJSONResponse{
				Body:       *prob,
				StatusCode: http.StatusPreconditionFailed,
			}, nil
		case errors.Is(err, domain.ErrProfileNotFound):
			return api.DeleteProfile404ApplicationProblemPlusJSONResponse(*prob), nil
		default:
			return api.DeleteProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 500}, nil
		}
	}
	return api.DeleteProfile204Response{}, nil
}
