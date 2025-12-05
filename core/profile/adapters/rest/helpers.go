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
	"fmt"
	"strconv"
	"strings"

	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
	"app/modules/api/serde"
	"app/modules/etag"

	"github.com/oapi-codegen/nullable"
	"github.com/oapi-codegen/runtime/types"
)

// mapProfile converts domain profile models to API response models.
func mapProfile(profiles []domain.Profile) []api.Profile {
	result := make([]api.Profile, 0)
	for _, p := range profiles {
		// TODO: static lint tool to check unmapped fields
		result = append(result, api.Profile{
			Id:        types.UUID(p.ID),
			Age:       serde.Ptr(strconv.Itoa(p.Age)),
			Name:      p.Name,
			CreatedAt: &p.CreatedAt,
			Email:     nullable.NewNullableWithValue(types.Email(p.Email)),
		})
	}
	return result
}

// buildEtagsMap creates a UUID → ETag mapping for all profiles in the response.
// This allows clients to update multiple items without N+1 GET requests.
func buildEtagsMap(profiles []domain.Profile) map[string]string {
	etags := make(map[string]string, len(profiles))
	for _, p := range profiles {
		etags[p.ID.String()] = etag.ETag(&p)
	}
	return etags
}

// computeCollectionETag creates a collection-level ETag by combining individual item ETags.
// Format: "collection:{pagination-info}:{combined-etags}"
func computeCollectionETag(profiles []domain.Profile, paginationInfo string) string {
	if len(profiles) == 0 {
		return fmt.Sprintf("collection:empty:%s", paginationInfo)
	}

	// Combine all individual ETags
	var etagBuilder strings.Builder
	for i, p := range profiles {
		if i > 0 {
			etagBuilder.WriteString(",")
		}
		etagBuilder.WriteString(etag.ETag(&p))
	}

	// Create a simple hash representation
	// In production, you might want to use a proper hash function
	combinedEtags := etagBuilder.String()
	return fmt.Sprintf("collection:%s:%s", paginationInfo, combinedEtags)
}
