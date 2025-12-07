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

package domain

import (
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
)

type (
	Application struct {
		reader ProfileReadStore
		writer ProfileWriteStore
		signer CursorSigner
	}

	// Profile is the domain model used by the application layer.
	Profile struct {
		ID        uuid.UUID
		Name      string
		Email     string
		Age       int
		CreatedAt time.Time

		Version int64
	}
)

func (p *Profile) V() string {
	return strconv.Itoa(int(p.Version))
}

const (
	ASC  CursorDirection = "asc"
	DESC CursorDirection = "desc"
)

type (
	CursorDirection string

	CursorPaginationToken struct {
		TTL       time.Time       `json:"ttl"`
		Direction CursorDirection `json:"direction"`

		Pivot struct {
			CreatedAt time.Time `json:"created_at"`
			ID        uuid.UUID `json:"id"`
		} `json:"pivot"`

		Signature string `json:"-"`
	}
)
