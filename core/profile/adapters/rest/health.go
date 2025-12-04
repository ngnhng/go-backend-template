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

	api "app/modules/api/profileapi/stdlib"
)

// Healthz returns 204 to indicate the service is healthy.
func (p *ProfileAPI) Healthz(ctx context.Context, request api.HealthzRequestObject) (api.HealthzResponseObject, error) {
	return api.Healthz204Response{}, nil
}
