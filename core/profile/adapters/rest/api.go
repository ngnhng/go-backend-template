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
	"app/core/profile/domain"
	api "app/modules/api/profileapi/stdlib"
)

// ProfileAPI implements the HTTP API handlers for profile operations.
// It acts as the REST adapter in the hexagonal architecture, translating
// HTTP requests into domain operations.
type ProfileAPI struct {
	app *domain.Application
}

// NewProfileService creates a new ProfileAPI instance with all dependencies.
func NewProfileService(reader domain.ProfileReadStore, writer domain.ProfileWriteStore, signer domain.CursorSigner) *ProfileAPI {
	return &ProfileAPI{
		app: domain.NewApp(reader, writer, signer),
	}
}

// Ensure ProfileAPI implements the generated StrictServerInterface
var _ api.StrictServerInterface = (*ProfileAPI)(nil)
