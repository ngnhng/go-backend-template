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

package persistence

import (
	"app/core/profile/domain"
	"database/sql"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type (
	// ProfileRow is the persistence entity shape used by storage adapters.
	ProfileRow struct {
		ID        uuid.UUID     `db:"id"`
		Version   sql.NullInt64 `db:"version_number"`
		Name      string        `db:"username"`
		Email     string        `db:"email"`
		Age       sql.NullInt32 `db:"age"`
		CreatedAt time.Time     `db:"created_at"`
		UpdatedAt time.Time     `db:"updated_at"`
		DeletedAt sql.NullTime  `db:"deleted_at"`
	}
)

// toProfile converts a ProfileRow to a domain Profile.
func toProfile(row ProfileRow) domain.Profile {
	return domain.Profile{
		ID:        row.ID,
		Name:      row.Name,
		Email:     row.Email,
		Age:       int(row.Age.Int32),
		CreatedAt: row.CreatedAt,
		Version:   row.Version.Int64,
	}
}

// profileTransformer implements bob's transformer interface for automatic row to domain conversion.
type profileTransformer struct{}

func (profileTransformer) TransformScanned(rows []ProfileRow) ([]domain.Profile, error) {
	out := make([]domain.Profile, len(rows))
	for i, r := range rows {
		out[i] = toProfile(r)
	}
	return out, nil
}

// wrapProfileError centralizes mapping of DB errors to domain errors.
func wrapProfileError(err error) error {
	if err == nil {
		return nil
	}

	// sql.ErrNoRows is expected in many flows (not found / precondition failed)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrProfileNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return domain.ErrDuplicateProfile
		case "40001": // serialization_failure
			return domain.ErrPrecondition
		}
	}

	return err
}
