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
	"app/db"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

// On pgx error handling:
// https://github.com/jackc/pgx/wiki/Error-Handling

// On pgx error handling:
// https://github.com/jackc/pgx/wiki/Error-Handling
var (
	ErrDuplicateEntry = errors.New("item with this identifier already exists")
)

type (
	PostgresProfilePersistence struct {
		TableName string
	}
)

var _ ProfilePersistence = (*PostgresProfilePersistence)(nil)

// GetProfilesByCursor implements ProfilePersistence (pivot-based).
func (p *PostgresProfilePersistence) GetProfilesByCursor(ctx context.Context, q db.Querier, pivotCreatedAt time.Time, pivotID uuid.UUID, dir CursorDirection, limit int) ([]Profile, error) {
	// Comparator direction relative to fixed presentation ORDER BY created_at DESC, id DESC
	comparator := "<"
	if dir == ASC {
		comparator = ">"
	}
	if limit <= 0 {
		return nil, ErrInvalidData
	}

	listQuery := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE deleted_at IS NULL
        	AND (created_at, id) %s ($1, $2)
        ORDER BY created_at DESC, id DESC
        LIMIT $3
    `, p.TableName, comparator)

	var profiles []Profile
	if err := sqlx.SelectContext(ctx, q, &profiles, listQuery, pivotCreatedAt, pivotID, limit); err != nil {
		slog.ErrorContext(ctx, "query error", slog.Any("error", err))
		return nil, fmt.Errorf("select profiles by cursor: %w", err)
	}
	return profiles, nil
}

// GetProfilesFirstPage returns the first page in cursor presentation order.
func (p *PostgresProfilePersistence) GetProfilesFirstPage(ctx context.Context, q db.Querier, limit int) ([]Profile, error) {
	if limit <= 0 {
		return nil, ErrInvalidData
	}
	listQuery := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC, id DESC
        LIMIT $1
    `, p.TableName)

	var profiles []Profile
	if err := sqlx.SelectContext(ctx, q, &profiles, listQuery, limit); err != nil {
		slog.ErrorContext(ctx, "query error", slog.Any("error", err))
		return nil, fmt.Errorf("select profiles first page: %w", err)
	}
	return profiles, nil
}

// GetProfilesByOffset implements ProfilePersistence.
func (p *PostgresProfilePersistence) GetProfilesByOffset(ctx context.Context, q db.Querier, limit int, offset int) ([]Profile, int, error) {
	listQuery := fmt.Sprintf(`
        SELECT id, username, email, age, created_at
        FROM %s
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC, id DESC
        LIMIT $1 OFFSET $2
    `, p.TableName)

	var profiles []Profile
	if err := sqlx.SelectContext(ctx, q, &profiles, listQuery, limit, offset); err != nil {
		slog.ErrorContext(ctx, "query error", slog.Any("error", err))
		return nil, 0, fmt.Errorf("select profiles: %w", err)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL`, p.TableName)
	var count int
	if err := sqlx.GetContext(ctx, q, &count, countQuery); err != nil {
		slog.ErrorContext(ctx, "count error", slog.Any("error", err))
		return nil, 0, fmt.Errorf("count profiles: %w", err)
	}

	return profiles, count, nil
}

func (p *PostgresProfilePersistence) CreateProfile(ctx context.Context, q db.Querier, username, email string) (*Profile, error) {
	query := fmt.Sprintf(`
		INSERT INTO %s (username, email)
		VALUES ($1, $2)
		RETURNING id, username, email, age, created_at;
		`, p.TableName)

	var createdProfile Profile
	slog.DebugContext(ctx, "debug query", slog.Any("query", query))
	if err := sqlx.GetContext(ctx, q, &createdProfile, query, username, email); err != nil {
		slog.DebugContext(ctx, "error persisting profile", slog.Any("error", err))
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.DebugContext(ctx, "duplicate profile", slog.Any("error", pgErr))
			return nil, ErrDuplicateEntry
		}
		return nil, fmt.Errorf("unexpected error creating profile: %w", err)
	}

	slog.DebugContext(ctx, "persisted profile", slog.Any("profile", fmt.Sprintf("%+v", createdProfile)))
	return &createdProfile, nil
}
