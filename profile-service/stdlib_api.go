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
	api "app/api/profileapi/stdlib"
	"app/api/serde"
	"app/db"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/oapi-codegen/nullable"
	"github.com/oapi-codegen/runtime/types"
)

type ProfileAPI struct {
	app *ProfileManager
}

var _ api.StrictServerInterface = (*ProfileAPI)(nil)

// CreateProfile implements profile_api.StrictServerInterface.
func (p *ProfileAPI) CreateProfile(ctx context.Context, request api.CreateProfileRequestObject) (api.CreateProfileResponseObject, error) {
	profile, err := p.app.CreateProfile(ctx, request.Body.Name, string(*request.Body.Email))
	if err != nil {
		prob := ProblemFromDomainError(err)
		slog.DebugContext(ctx, "domain error", slog.Any("error", err))
		if errors.Is(err, ErrInvalidData) {
			WithInvalidParam("name", "invalid value")(prob)
			return api.CreateProfile422ApplicationProblemPlusJSONResponse(*prob), nil
		}
		status := 500
		if errors.Is(err, ErrDuplicateProfile) {
			status = 409
		}
		return api.CreateProfiledefaultApplicationProblemPlusJSONResponse{Body: *prob, StatusCode: status}, nil
	}

	resp := api.SuccessProfile{
		Data: api.Profile{
			Id:   [16]byte(profile.ID.Bytes()),
			Name: profile.Name},
	}
	return api.CreateProfile201JSONResponse{
		Body:    resp,
		Headers: api.CreateProfile201ResponseHeaders{Location: fmt.Sprintf("/v1/profiles/%s", profile.ID)},
	}, nil
}

// DeleteProfile implements profile_api.StrictServerInterface.
func (p *ProfileAPI) DeleteProfile(ctx context.Context, request api.DeleteProfileRequestObject) (api.DeleteProfileResponseObject, error) {
	panic("unimplemented")
}

// GetProfileById implements profile_api.StrictServerInterface.
func (p *ProfileAPI) GetProfileById(ctx context.Context, request api.GetProfileByIdRequestObject) (api.GetProfileByIdResponseObject, error) {
	panic("unimplemented")
}

// Healthz implements profile_api.StrictServerInterface.
func (p *ProfileAPI) Healthz(ctx context.Context, request api.HealthzRequestObject) (api.HealthzResponseObject, error) {
	panic("unimplemented")
}

// ListProfiles implements profile_api.StrictServerInterface.
func (p *ProfileAPI) ListProfiles(ctx context.Context, request api.ListProfilesRequestObject) (api.ListProfilesResponseObject, error) {
	// Determine which pagination mode is requested and ensure completeness.
	offsetProvided := request.Params.Page != nil || request.Params.PageSize != nil
	cursorProvided := request.Params.After != nil || request.Params.Before != nil || request.Params.Limit != nil
	offsetComplete := request.Params.Page != nil && request.Params.PageSize != nil
	// cursor mode requires limit and at most one of after or before
	hasAfter := request.Params.After != nil
	hasBefore := request.Params.Before != nil
	limitProvided := request.Params.Limit != nil
	cursorComplete := limitProvided && !(hasAfter && hasBefore)

	slog.DebugContext(ctx,
		"pagination params",
		slog.Any("offsetProvided", offsetProvided),
		slog.Any("cursorProvided", cursorProvided),
		slog.Any("offsetComplete", offsetComplete),
		slog.Any("cursorComplete", cursorComplete),
	)

	// Invalid when: both complete, neither provided, or any incomplete set.
	if (offsetComplete && cursorComplete) || (!offsetProvided && !cursorProvided) || (offsetProvided && !offsetComplete) || (cursorProvided && !cursorComplete) {
		prob := BadRequestProblem("Provide either page+pageSize or cursor+limit (complete pair)")
		return api.ListProfiles400ApplicationProblemPlusJSONResponse{
			ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob),
		}, nil
	}

	// --- offset based ---
	if offsetComplete {
		limit := *request.Params.PageSize
		page := *request.Params.Page
		slog.DebugContext(ctx, "using offset pagination", slog.Any("page", page), slog.Any("pageSize", limit))

		profiles, count, err := p.app.GetProfilesByOffset(ctx, page, limit)
		if err != nil {
			return api.ListProfilesdefaultApplicationProblemPlusJSONResponse{
				Body:       *InternalProblem("query failed"),
				StatusCode: 500,
			}, nil
		}

		pages := 0
		if limit > 0 {
			pages = (count + limit - 1) / limit
		}
		meta := api.PaginationMeta{}
		_ = meta.FromOffsetMeta(api.OffsetMeta{
			Page:       page,
			PageSize:   limit,
			TotalItems: count,
			TotalPages: pages,
			Links: &struct {
				Next *string `json:"next,omitempty"`
				Prev *string `json:"prev,omitempty"`
			}{
				Next: serde.Ptr(""),
				Prev: serde.Ptr(""),
			},
		})
		return &api.ListProfiles200JSONResponse{
			Body: api.SuccessProfileList{
				Data: mapProfile(profiles),
				Meta: meta,
			},
			Headers: api.ListProfiles200ResponseHeaders{
				Link: "",
			},
		}, nil
	}

	// --- cursor based ---
	// Only reached when cursorComplete is true (limit provided)
	limit := *request.Params.Limit
	// Initial page: no before/after
	if !hasAfter && !hasBefore {
		profiles, err := p.app.GetProfilesFirstPage(ctx, limit)
		if err != nil {
			return api.ListProfilesdefaultApplicationProblemPlusJSONResponse{
				Body:       *InternalProblem("query failed"),
				StatusCode: 500,
			}, nil
		}
		var nextStr, prevStr *string
		if len(profiles) > 0 {
			last := profiles[len(profiles)-1]
			// Newest first, so there is no "prev" set for initial page
			n := p.app.makeCursorFromProfile(last, DESC, 24*time.Hour)
			nextStr = serde.Ptr(n)
			// prev remains nil on initial page
		}
		meta := api.PaginationMeta{}
		_ = meta.FromCursorMeta(api.CursorMeta{
			Limit:      limit,
			NextCursor: nextStr,
			PrevCursor: prevStr,
		})
		return &api.ListProfiles200JSONResponse{
			Body: api.SuccessProfileList{
				Data: mapProfile(profiles),
				Meta: meta,
			},
			Headers: api.ListProfiles200ResponseHeaders{Link: ""},
		}, nil
	}

	var inCursor string
	if hasAfter {
		inCursor = *request.Params.After
	} else {
		inCursor = *request.Params.Before
	}
	slog.DebugContext(ctx, "using cursor pagination", slog.Any("limit", limit))

	profiles, _, err := p.app.GetProfilesByCursor(ctx, inCursor, limit)
	if err != nil {
		// Treat invalid cursor as 400 with invalid param detail
		prob := BadRequestProblem("invalid cursor")
		if hasAfter {
			WithInvalidParam("after", "invalid value")(prob)
		} else {
			WithInvalidParam("before", "invalid value")(prob)
		}
		return api.ListProfiles400ApplicationProblemPlusJSONResponse{
			ProblemResponseApplicationProblemPlusJSONResponse: api.ProblemResponseApplicationProblemPlusJSONResponse(*prob),
		}, nil
	}

	// Build cursor meta with next/prev using page edges
	var nextStr, prevStr *string
	if len(profiles) > 0 {
		first := profiles[0]
		last := profiles[len(profiles)-1]
		n := p.app.makeCursorFromProfile(last, DESC, 24*time.Hour)
		pcur := p.app.makeCursorFromProfile(first, ASC, 24*time.Hour)
		nextStr = serde.Ptr(n)
		prevStr = serde.Ptr(pcur)
	}
	meta := api.PaginationMeta{}
	_ = meta.FromCursorMeta(api.CursorMeta{
		Limit:      limit,
		NextCursor: nextStr,
		PrevCursor: prevStr,
	})
	return &api.ListProfiles200JSONResponse{
		Body:    api.SuccessProfileList{Data: mapProfile(profiles), Meta: meta},
		Headers: api.ListProfiles200ResponseHeaders{Link: ""},
	}, nil

}

// ModifyProfile implements profile_api.StrictServerInterface.
func (p *ProfileAPI) ModifyProfile(ctx context.Context, request api.ModifyProfileRequestObject) (api.ModifyProfileResponseObject, error) {
	panic("unimplemented")
}

// UpdateProfile implements profile_api.StrictServerInterface.
func (p *ProfileAPI) UpdateProfile(ctx context.Context, request api.UpdateProfileRequestObject) (api.UpdateProfileResponseObject, error) {
	panic("unimplemented")
}

func NewProfileService(pool db.ConnectionPool, persistence ProfilePersistence, signer CursorSigner) *ProfileAPI {
	return &ProfileAPI{
		app: newApp(pool, persistence, signer),
	}
}

func mapProfile(profiles []Profile) []api.Profile {
	result := make([]api.Profile, 0)
	for _, p := range profiles {
		var agePtr *string
		if p.Age.Valid {
			s := strconv.Itoa(int(p.Age.Int32))
			agePtr = serde.Ptr(s)
		}
		// TODO: static lint tool to check unmapped fields
		result = append(result, api.Profile{
			Id:        types.UUID(p.ID),
			Age:       agePtr,
			Name:      p.Name,
			CreatedAt: &p.CreatedAt,
			Email:     nullable.NewNullableWithValue(types.Email(p.Email)),
		})
	}
	return result
}
