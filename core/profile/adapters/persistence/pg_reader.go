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
	"context"
	"fmt"
	"log/slog"
	"time"

	"app/core/profile/domain"
	"app/modules/db"

	"github.com/gofrs/uuid/v5"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/stephenafamo/scan"
)

var _ domain.ProfileReadStore = (*PostgresProfileReader)(nil)

type (
	PostgresProfileReader struct {
		table string
		pool  db.ReaderConnectionManager // calls Reader() at runtime
	}
)

// NewPostgresProfileReader creates a new reader that calls Reader() at runtime for load balancing.
//
// This approach uses dynamic queries instead of prepared statements for reads.
// Trade-offs:
//   - Supports runtime replica selection (load balancing across multiple replicas)
//   - Automatic failover if a replica goes down
//   - Simple implementation
//   - Slightly slower than prepared statements (but read queries are typically fast)
//
// If read performance is critical and you have a single replica, consider using prepared
// statements bound to that replica. For most use cases, dynamic queries are sufficient.
func NewPostgresProfileReader(pool db.ReaderConnectionManager, table string) *PostgresProfileReader {
	return &PostgresProfileReader{
		table: table,
		pool:  pool,
	}
}

// GetProfilesByCursor implements ProfileReadStore (pivot-based cursor).
// Calls pool.Reader() at runtime for replica load balancing.
func (r *PostgresProfileReader) GetProfilesByCursor(
	ctx context.Context,
	pivotCreatedAt time.Time,
	pivotID uuid.UUID,
	dir domain.CursorDirection,
	limit int,
) ([]domain.Profile, error) {
	if limit <= 0 {
		return nil, domain.ErrInvalidData
	}

	// Comparator relative to ORDER BY created_at DESC, id DESC
	comparator := "<"
	if dir == domain.ASC {
		comparator = ">"
	}

	raw := fmt.Sprintf(`
		SELECT id, username, email, age, created_at, version_number
		FROM %s
		WHERE deleted_at IS NULL
		  AND (created_at, id) %s ($1, $2)
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	`, r.table, comparator)

	q := psql.RawQuery(raw, pivotCreatedAt, pivotID, limit)
	rows, err := bob.Allx[profileTransformer](ctx, r.pool.Reader(), q, scan.StructMapper[ProfileRow]())
	if err != nil {
		slog.ErrorContext(ctx, "GetProfilesByCursor query error", slog.Any("err", err))
		return nil, wrapProfileError(err)
	}
	return rows, nil
}

func (r *PostgresProfileReader) GetProfilesFirstPage(ctx context.Context, limit int) ([]domain.Profile, error) {
	if limit <= 0 {
		return nil, domain.ErrInvalidData
	}

	query := psql.Select(
		sm.Columns("id", "username", "email", "age", "created_at", "version_number"),
		sm.From(r.table),
		sm.Where(psql.Quote("deleted_at").IsNull()),
		sm.OrderBy("created_at").Desc(),
		sm.OrderBy("id").Desc(),
		sm.Limit(limit),
	)

	profiles, err := bob.Allx[profileTransformer](ctx, r.pool.Reader(), query, scan.StructMapper[ProfileRow]())
	if err != nil {
		slog.ErrorContext(ctx, "GetProfilesFirstPage error", slog.Any("err", err))
		return nil, wrapProfileError(err)
	}
	return profiles, nil
}

func (r *PostgresProfileReader) GetProfilesByOffset(
	ctx context.Context,
	limit, offset int,
) ([]domain.Profile, int, error) {
	if limit <= 0 || offset < 0 {
		return nil, 0, domain.ErrInvalidData
	}

	listQuery := psql.Select(
		sm.Columns("id", "username", "email", "age", "created_at", "version_number"),
		sm.From(r.table),
		sm.Where(psql.Quote("deleted_at").IsNull()),
		sm.OrderBy("created_at").Desc(),
		sm.OrderBy("id").Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	)

	profiles, err := bob.Allx[profileTransformer](ctx, r.pool.Reader(), listQuery, scan.StructMapper[ProfileRow]())
	if err != nil {
		slog.ErrorContext(ctx, "GetProfilesByOffset query error", slog.Any("err", err))
		return nil, 0, wrapProfileError(err)
	}

	countQuery := psql.Select(
		sm.Columns("COUNT(*)"),
		sm.From(r.table),
		sm.Where(psql.Quote("deleted_at").IsNull()),
	)

	count, err := bob.One(ctx, r.pool.Reader(), countQuery, scan.SingleColumnMapper[int])
	if err != nil {
		slog.ErrorContext(ctx, "GetProfilesByOffset count error", slog.Any("err", err))
		return nil, 0, wrapProfileError(err)
	}

	return profiles, count, nil
}

func (r *PostgresProfileReader) GetProfileByID(ctx context.Context, id uuid.UUID) (*domain.Profile, error) {
	query := psql.Select(
		sm.Columns("id", "username", "email", "age", "created_at", "version_number"),
		sm.From(r.table),
		sm.Where(psql.Quote("id").EQ(psql.Arg(id))),
		sm.Where(psql.Quote("deleted_at").IsNull()),
	)

	row, err := bob.One(ctx, r.pool.Reader(), query, scan.StructMapper[ProfileRow]())
	if err != nil {
		return nil, wrapProfileError(err)
	}
	prof := toProfile(row)
	return &prof, nil
}
