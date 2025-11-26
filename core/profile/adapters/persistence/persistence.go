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

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"app/core/profile/domain"
	"app/db"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// On pgx error handling:
// https://github.com/jackc/pgx/wiki/Error-Handling
var ()

var _ domain.ProfilePersistence = (*PostgresProfilePersistence)(nil)

type (
	PostgresProfilePersistence struct {
		TableName string
	}

	// ProfileRow is the persistence entity shape used by storage adapters.
	ProfileRow struct {
		ID        uuid.UUID     `db:"id"`
		Name      string        `db:"username"`
		Email     string        `db:"email"`
		Age       sql.NullInt32 `db:"age"`
		CreatedAt time.Time     `db:"created_at"`
	}
)

func toProfile(row ProfileRow) domain.Profile {
	return domain.Profile(row)
}

func toProfiles(rows []ProfileRow) []domain.Profile {
	out := make([]domain.Profile, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProfile(r))
	}
	return out
}

// GetProfilesByCursor implements ProfilePersistence (pivot-based).
func (p *PostgresProfilePersistence) GetProfilesByCursor(ctx context.Context, q db.Querier, pivotCreatedAt time.Time, pivotID uuid.UUID, dir domain.CursorDirection, limit int) ([]domain.Profile, error) {
	// Comparator direction relative to fixed presentation ORDER BY created_at DESC, id DESC
	comparator := "<"
	if dir == domain.ASC {
		comparator = ">"
	}
	if limit <= 0 {
		return nil, domain.ErrInvalidData
	}

	listQuery := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE deleted_at IS NULL
        	AND (created_at, id) %s ($1, $2)
        ORDER BY created_at DESC, id DESC
        LIMIT $3
    `, p.TableName, comparator)

	var rows []ProfileRow
	if err := sqlx.SelectContext(ctx, q, &rows, listQuery, pivotCreatedAt, pivotID, limit); err != nil {
		slog.ErrorContext(ctx, "query error", slog.Any("error", err))
		return nil, fmt.Errorf("select profiles by cursor: %w", err)
	}
	return toProfiles(rows), nil
}

// GetProfilesFirstPage returns the first page in cursor presentation order.
func (p *PostgresProfilePersistence) GetProfilesFirstPage(ctx context.Context, q db.Querier, limit int) ([]domain.Profile, error) {
	if limit <= 0 {
		return nil, domain.ErrInvalidData
	}
	listQuery := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC, id DESC
        LIMIT $1
    `, p.TableName)

	var rows []ProfileRow
	if err := sqlx.SelectContext(ctx, q, &rows, listQuery, limit); err != nil {
		slog.ErrorContext(ctx, "query error", slog.Any("error", err))
		return nil, fmt.Errorf("select profiles first page: %w", err)
	}
	return toProfiles(rows), nil
}

// GetProfilesByOffset implements ProfilePersistence.
func (p *PostgresProfilePersistence) GetProfilesByOffset(ctx context.Context, q db.Querier, limit int, offset int) ([]domain.Profile, int, error) {
	listQuery := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC, id DESC
        LIMIT $1 OFFSET $2
    `, p.TableName)

	var rows []ProfileRow
	if err := sqlx.SelectContext(ctx, q, &rows, listQuery, limit, offset); err != nil {
		slog.ErrorContext(ctx, "query error", slog.Any("error", err))
		return nil, 0, fmt.Errorf("select profiles: %w", err)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL`, p.TableName)
	var count int
	if err := sqlx.GetContext(ctx, q, &count, countQuery); err != nil {
		slog.ErrorContext(ctx, "count error", slog.Any("error", err))
		return nil, 0, fmt.Errorf("count profiles: %w", err)
	}

	return toProfiles(rows), count, nil
}

func (p *PostgresProfilePersistence) CreateProfile(ctx context.Context, q db.Querier, username, email string) (*domain.Profile, error) {
	query := fmt.Sprintf(`
		INSERT INTO %s (username, email)
		VALUES ($1, $2)
		RETURNING id, username, email, age, created_at;
		`, p.TableName)

	var createdProfile ProfileRow
	slog.DebugContext(ctx, "debug query", slog.Any("query", query))
	if err := sqlx.GetContext(ctx, q, &createdProfile, query, username, email); err != nil {
		slog.DebugContext(ctx, "error persisting profile", slog.Any("error", err))
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.DebugContext(ctx, "duplicate profile", slog.Any("error", pgErr))
			return nil, domain.ErrDuplicateProfile
		}
		return nil, fmt.Errorf("unexpected error creating profile: %w", err)
	}

	slog.DebugContext(ctx, "persisted profile", slog.Any("profile", fmt.Sprintf("%+v", createdProfile)))
	prof := toProfile(createdProfile)
	return &prof, nil
}

// GetProfileByID fetches a single non-deleted profile.
func (p *PostgresProfilePersistence) GetProfileByID(ctx context.Context, q db.Querier, id uuid.UUID) (*domain.Profile, error) {
	query := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE id = $1 AND deleted_at IS NULL
    `, p.TableName)
	var profRow ProfileRow
	if err := sqlx.GetContext(ctx, q, &profRow, query, id); err != nil {
		return nil, err
	}
	prof := toProfile(profRow)
	return &prof, nil
}

// UpdateProfile updates username and optionally email, returning the updated entity.
func (p *PostgresProfilePersistence) UpdateProfile(ctx context.Context, q db.Querier, id uuid.UUID, name string, email *string) (*domain.Profile, error) {
	if len(name) == 0 {
		return nil, domain.ErrInvalidData
	}
	var (
		query string
		args  []any
	)
	if email != nil {
		query = fmt.Sprintf(`
            UPDATE %s
            SET username = $2, email = $3
            WHERE id = $1 AND deleted_at IS NULL
            RETURNING id, username, email, age, created_at
        `, p.TableName)
		args = []any{id, name, *email}
	} else {
		query = fmt.Sprintf(`
            UPDATE %s
            SET username = $2
            WHERE id = $1 AND deleted_at IS NULL
            RETURNING id, username, email, age, created_at
        `, p.TableName)
		args = []any{id, name}
	}
	var profRow ProfileRow
	if err := sqlx.GetContext(ctx, q, &profRow, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrDuplicateProfile
		}
		return nil, err
	}
	prof := toProfile(profRow)
	return &prof, nil
}

// DeleteProfile performs a soft delete; returns sql.ErrNoRows if not found.
func (p *PostgresProfilePersistence) DeleteProfile(ctx context.Context, q db.Querier, id uuid.UUID) error {
	query := fmt.Sprintf(`
        UPDATE %s SET deleted_at = CURRENT_TIMESTAMP
        WHERE id = $1 AND deleted_at IS NULL
        RETURNING id
    `, p.TableName)
	var ret uuid.UUID
	if err := sqlx.GetContext(ctx, q, &ret, query, id); err != nil {
		return err
	}
	return nil
}

// ModifyProfile performs partial updates based on provided fields.
func (p *PostgresProfilePersistence) ModifyProfile(ctx context.Context, q db.Querier, id uuid.UUID, nameSet bool, nameNull bool, nameVal string, ageSet bool, ageNull bool, ageVal int32, emailSet bool, emailVal string) (*domain.Profile, error) {
	if !nameSet && !ageSet && !emailSet {
		return nil, domain.ErrInvalidData
	}
	sets := []string{}
	args := []any{id}
	idx := 2
	if nameSet {
		if nameNull {
			sets = append(sets, "username = NULL")
		} else {
			sets = append(sets, fmt.Sprintf("username = $%d", idx))
			args = append(args, nameVal)
			idx++
		}
	}
	if ageSet {
		if ageNull {
			sets = append(sets, "age = NULL")
		} else {
			sets = append(sets, fmt.Sprintf("age = $%d", idx))
			args = append(args, ageVal)
			idx++
		}
	}
	if emailSet {
		sets = append(sets, fmt.Sprintf("email = $%d", idx))
		args = append(args, emailVal)
		idx++
	}
	query := fmt.Sprintf(`
        UPDATE %s
        SET %s
        WHERE id = $1 AND deleted_at IS NULL
        RETURNING id, username, email, age, created_at
    `, p.TableName, strings.Join(sets, ", "))

	var profRow ProfileRow
	if err := sqlx.GetContext(ctx, q, &profRow, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrDuplicateProfile
		}
		return nil, err
	}
	prof := toProfile(profRow)
	return &prof, nil
}
