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

// ModifyProfile performs a partial update of a profile (PATCH semantics).
// Requires If-Match header with current ETag for optimistic concurrency control.
// Supports nullable fields with tri-state logic (unset/null/value).
// Returns 200 on success, 412 if version mismatch, 404 if not found.
func (p *ProfileAPI) ModifyProfile(ctx context.Context, request api.ModifyProfileRequestObject) (api.ModifyProfileResponseObject, error) {
	uid, err := uuid.FromBytes(request.Id[:])
	if err != nil {
		prob := BadRequestProblem("invalid id")
		WithInvalidParam("id", "invalid value")(prob)
		return api.ModifyProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	// Parse version from ETag without querying database
	ifMatch := string(request.Params.IfMatch)
	if ifMatch == "" {
		prob := BadRequestProblem("missing if-match header")
		WithInvalidParam("If-Match", "header is required")(prob)
		return api.ModifyProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	versionStr, err := etag.ParseETag(ifMatch)
	if err != nil {
		prob := BadRequestProblem("invalid etag format")
		WithInvalidParam("If-Match", "invalid etag format")(prob)
		return api.ModifyProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		prob := BadRequestProblem("invalid etag version")
		WithInvalidParam("If-Match", "invalid version in etag")(prob)
		return api.ModifyProfile400ApplicationProblemPlusJSONResponse{ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob)}, nil
	}

	// Compute tri-state updates
	nameSet, nameNull, nameVal := false, false, ""
	ageSet, ageNull := false, false
	var ageValInt32 int32
	emailSet, emailVal := false, ""

	if request.Body != nil {
		// name: nullable string
		if request.Body.Name.IsSpecified() {
			nameSet = true
			if request.Body.Name.IsNull() {
				nameNull = true
			} else {
				v := request.Body.Name.MustGet()
				nameVal = v
			}
		}
		// age: nullable string containing integer (1..150)
		if request.Body.Age.IsSpecified() {
			ageSet = true
			if request.Body.Age.IsNull() {
				ageNull = true
			} else {
				v := request.Body.Age.MustGet()
				if v == "" {
					prob := ValidationProblem("validation failed")
					WithInvalidParam("age", "invalid value")(prob)
					return api.ModifyProfile422ApplicationProblemPlusJSONResponse(*prob), nil
				}
				n, perr := strconv.Atoi(v)
				if perr != nil || n < 1 || n > 150 {
					prob := ValidationProblem("validation failed")
					WithInvalidParam("age", "invalid value")(prob)
					return api.ModifyProfile422ApplicationProblemPlusJSONResponse(*prob), nil
				}
				ageValInt32 = int32(n)
			}
		}
		// email: regular optional update, null not accepted
		if request.Body.Email != nil {
			emailSet = true
			emailVal = string(*request.Body.Email)
		}
	}

	if !nameSet && !ageSet && !emailSet {
		prob := ValidationProblem("validation failed")
		WithInvalidParam("body", "no valid fields to update")(prob)
		return api.ModifyProfile422ApplicationProblemPlusJSONResponse(*prob), nil
	}

	updated, err := p.app.ModifyProfile(ctx, uid, version, nameSet, nameNull, nameVal, ageSet, ageNull, ageValInt32, emailSet, emailVal)
	if err != nil {
		prob := ProblemFromDomainError(err)
		switch {
		case errors.Is(err, domain.ErrInvalidData):
			WithInvalidParam("body", "no valid fields to update")(prob)
			return api.ModifyProfile422ApplicationProblemPlusJSONResponse(*prob), nil
		case errors.Is(err, domain.ErrDuplicateProfile):
			return api.ModifyProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 409}, nil
		case errors.Is(err, domain.ErrPrecondition):
			// On version mismatch, fetch latest to return current ETag in response
			latest, fetchErr := p.app.GetProfileByID(ctx, uid)
			etagVal := ifMatch
			if fetchErr == nil {
				etagVal = etag.ETag(latest)
			}
			return api.ModifyProfile412ApplicationProblemPlusJSONResponse{
				PreconditionFailedResponseApplicationProblemPlusJSONResponse: api.PreconditionFailedResponseApplicationProblemPlusJSONResponse{
					Body:    *prob,
					Headers: api.PreconditionFailedResponseResponseHeaders{ETag: etagVal},
				},
			}, nil
		case errors.Is(err, domain.ErrProfileNotFound):
			return api.ModifyProfile404ApplicationProblemPlusJSONResponse(*prob), nil
		default:
			return api.ModifyProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: 500}, nil
		}
	}
	resp := api.SuccessProfile{Data: mapProfile([]domain.Profile{*updated})[0]}
	return api.ModifyProfile200JSONResponse(resp), nil
}
