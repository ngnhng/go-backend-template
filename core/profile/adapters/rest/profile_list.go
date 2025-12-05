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
	"fmt"
	"log/slog"
	"time"

	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
	"app/modules/api/serde"
)

// ListProfiles retrieves a paginated list of profiles.
// Supports both offset-based (page/pageSize) and cursor-based (after/before/limit) pagination.
// Returns collection ETag in header and per-item ETags in metadata.
func (p *ProfileAPI) ListProfiles(ctx context.Context, request api.ListProfilesRequestObject) (api.ListProfilesResponseObject, error) {
	// Determine which pagination mode is requested and ensure completeness.
	offsetProvided := request.Params.Page != nil || request.Params.PageSize != nil
	cursorProvided := request.Params.After != nil || request.Params.Before != nil || request.Params.Limit != nil
	offsetComplete := request.Params.Page != nil && request.Params.PageSize != nil
	// cursor mode requires limit and at most one of after or before
	hasAfter := request.Params.After != nil
	hasBefore := request.Params.Before != nil
	limitProvided := request.Params.Limit != nil
	cursorComplete := limitProvided && (!hasAfter || !hasBefore)

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
		etagsMap := buildEtagsMap(profiles)
		meta := api.PaginationMeta{}
		_ = meta.FromOffsetMeta(api.OffsetMeta{
			Page:       page,
			PageSize:   limit,
			TotalItems: count,
			TotalPages: pages,
			Etags:      &etagsMap,
			Links: &struct {
				Next *string `json:"next,omitempty"`
				Prev *string `json:"prev,omitempty"`
			}{
				Next: serde.Ptr(""),
				Prev: serde.Ptr(""),
			},
		})
		collectionEtag := computeCollectionETag(profiles, fmt.Sprintf("offset:p%d:ps%d", page, limit))
		return &api.ListProfiles200JSONResponse{
			Body: api.SuccessProfileList{
				Data: mapProfile(profiles),
				Meta: meta,
			},
			Headers: api.ListProfiles200ResponseHeaders{
				Link: "",
				ETag: collectionEtag,
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
			n := p.app.MakeCursorFromProfile(last, domain.DESC, 24*time.Hour)
			nextStr = serde.Ptr(n)
			// prev remains nil on initial page
		}
		etagsMap := buildEtagsMap(profiles)
		meta := api.PaginationMeta{}
		_ = meta.FromCursorMeta(api.CursorMeta{
			Limit:      limit,
			NextCursor: nextStr,
			PrevCursor: prevStr,
			Etags:      &etagsMap,
		})
		collectionEtag := computeCollectionETag(profiles, fmt.Sprintf("cursor:first:l%d", limit))
		return &api.ListProfiles200JSONResponse{
			Body: api.SuccessProfileList{
				Data: mapProfile(profiles),
				Meta: meta,
			},
			Headers: api.ListProfiles200ResponseHeaders{
				Link: "",
				ETag: collectionEtag,
			},
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
		n := p.app.MakeCursorFromProfile(last, domain.DESC, 24*time.Hour)
		pcur := p.app.MakeCursorFromProfile(first, domain.ASC, 24*time.Hour)
		nextStr = serde.Ptr(n)
		prevStr = serde.Ptr(pcur)
	}
	etagsMap := buildEtagsMap(profiles)
	meta := api.PaginationMeta{}
	_ = meta.FromCursorMeta(api.CursorMeta{
		Limit:      limit,
		NextCursor: nextStr,
		PrevCursor: prevStr,
		Etags:      &etagsMap,
	})
	direction := "after"
	if hasBefore {
		direction = "before"
	}
	collectionEtag := computeCollectionETag(profiles, fmt.Sprintf("cursor:%s:l%d", direction, limit))
	return &api.ListProfiles200JSONResponse{
		Body: api.SuccessProfileList{Data: mapProfile(profiles), Meta: meta},
		Headers: api.ListProfiles200ResponseHeaders{
			Link: "",
			ETag: collectionEtag,
		},
	}, nil
}
