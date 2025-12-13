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

package pg

import (
	"context"
	"fmt"
	"time"

	"app/core/profile/domain"
	"app/modules/db"

	"github.com/gofrs/uuid/v5"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/im"
	"github.com/stephenafamo/bob/dialect/psql/um"
	"github.com/stephenafamo/scan"
)

var _ domain.ProfileWriteStore = (*PostgresProfileWriter)(nil)

type (
	PostgresProfileWriter struct {
		table string
		db    *bob.DB // for prepared statements on primary
		txm   db.TxManager

		createStmt bob.QueryStmt[createProfileArgs, ProfileRow, []ProfileRow]
		updateStmt bob.QueryStmt[updateProfileArgs, ProfileRow, []ProfileRow]
		deleteStmt bob.QueryStmt[deleteProfileArgs, uuid.UUID, []uuid.UUID]
	}

	// Arg types for write operations
	createProfileArgs struct {
		Username string `db:"username"`
		Email    string `db:"email"`
	}

	updateProfileArgs struct {
		ID       uuid.UUID `db:"id"`
		Username string    `db:"username"`
		Email    string    `db:"email"`
		Version  int64     `db:"version_number"`
	}

	deleteProfileArgs struct {
		ID      uuid.UUID `db:"id"`
		Version int64     `db:"version_number"`
	}
)

var _ bob.Executor = (*bob.DB)(nil)

// NewPostgresProfileWriter creates a new writer with prepared statements bound to the primary.
func NewPostgresProfileWriter(ctx context.Context, pool db.ConnectionPool, table string) (*PostgresProfileWriter, error) {
	primary := pool.Writer().(bob.DB)

	w := &PostgresProfileWriter{
		table: table,
		db:    &primary,
		txm:   pool,
	}

	// INSERT INTO ... RETURNING ...
	insertQuery := psql.Insert(
		im.Into(table, "username", "email"),
		im.Values(
			bob.Named("username"),
			bob.Named("email"),
		),
		im.Returning("id", "username", "email", "age", "created_at", "version_number"),
	)

	createStmt, err := bob.PrepareQuery[createProfileArgs](ctx, primary, insertQuery, scan.StructMapper[ProfileRow]())
	if err != nil {
		return nil, fmt.Errorf("prepare create profile: %w", err)
	}
	w.createStmt = createStmt

	// UPDATE ... SET username = :username, email = :email, version_number = version_number + 1
	updateQuery := psql.Update(
		um.Table(table),
		um.SetCol("username").To(bob.Named("username")),
		um.SetCol("email").To(bob.Named("email")),
		um.SetCol("version_number").To(psql.Raw("version_number + 1")),
		um.Where(psql.Quote("id").EQ(bob.Named("id"))),
		um.Where(psql.Quote("deleted_at").IsNull()),
		um.Where(psql.Quote("version_number").EQ(bob.Named("version_number"))),
		um.Returning("id", "username", "email", "age", "created_at", "version_number"),
	)

	updateStmt, err := bob.PrepareQuery[updateProfileArgs](ctx, primary, updateQuery, scan.StructMapper[ProfileRow]())
	if err != nil {
		return nil, fmt.Errorf("prepare update profile: %w", err)
	}
	w.updateStmt = updateStmt

	// Soft delete with optimistic concurrency
	deleteQuery := psql.Update(
		um.Table(table),
		um.SetCol("deleted_at").To(psql.Raw("CURRENT_TIMESTAMP")),
		um.SetCol("version_number").To(psql.Raw("version_number + 1")),
		um.Where(psql.Quote("id").EQ(bob.Named("id"))),
		um.Where(psql.Quote("deleted_at").IsNull()),
		um.Where(psql.Quote("version_number").EQ(bob.Named("version_number"))),
		um.Returning("id"),
	)

	deleteStmt, err := bob.PrepareQuery[deleteProfileArgs](ctx, primary, deleteQuery, scan.SingleColumnMapper[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("prepare delete profile: %w", err)
	}
	w.deleteStmt = deleteStmt

	return w, nil
}

// CreateProfile implements ProfileWriteStore (non-transactional).
func (w *PostgresProfileWriter) CreateProfile(ctx context.Context, username, email string) (*domain.Profile, error) {
	row, err := w.createStmt.One(ctx, createProfileArgs{
		Username: username,
		Email:    email,
	})
	if err != nil {
		return nil, wrapProfileError(err)
	}
	p := toProfile(row)
	return &p, nil
}

// UpdateProfile implements ProfileWriteStore (non-transactional).
func (w *PostgresProfileWriter) UpdateProfile(ctx context.Context, params *domain.UpdateProfileParams) (*domain.Profile, error) {
	row, err := w.updateStmt.One(ctx, updateProfileArgs{
		ID:       params.ID,
		Username: params.Name,
		Email:    params.Email,
		Version:  params.Version,
	})
	if err != nil {
		return nil, wrapProfileError(err)
	}
	p := toProfile(row)
	return &p, nil
}

// DeleteProfile implements ProfileWriteStore (non-transactional).
func (w *PostgresProfileWriter) DeleteProfile(ctx context.Context, id uuid.UUID, version int64) error {
	_, err := w.deleteStmt.One(ctx, deleteProfileArgs{
		ID:      id,
		Version: version,
	})
	if err != nil {
		return wrapProfileError(err)
	}
	return nil
}

// ModifyProfile implements ProfileWriteStore (non-transactional).
// This is left unprepared because the SET clause is truly dynamic.
func (w *PostgresProfileWriter) ModifyProfile(
	ctx context.Context,
	id uuid.UUID,
	version int64,
	nameSet, nameNull bool, nameVal string,
	ageSet, ageNull bool, ageVal int32,
	emailSet bool, emailVal string,
) (*domain.Profile, error) {
	if !nameSet && !ageSet && !emailSet {
		return nil, domain.ErrInvalidData
	}

	query := psql.Update(
		um.Table(w.table),
		um.Where(psql.Quote("id").EQ(psql.Arg(id))),
		um.Where(psql.Quote("deleted_at").IsNull()),
		um.Where(psql.Quote("version_number").EQ(psql.Arg(version))),
	)

	// Conditionally add SET clauses
	if nameSet {
		if nameNull {
			query.Apply(um.SetCol("username").To(psql.Raw("NULL")))
		} else {
			query.Apply(um.SetCol("username").To(psql.Arg(nameVal)))
		}
	}

	if ageSet {
		if ageNull {
			query.Apply(um.SetCol("age").To(psql.Raw("NULL")))
		} else {
			query.Apply(um.SetCol("age").To(psql.Arg(ageVal)))
		}
	}

	if emailSet {
		query.Apply(um.SetCol("email").To(psql.Arg(emailVal)))
	}

	// Always increment version for optimistic locking
	query.Apply(
		um.SetCol("version_number").To(psql.Raw("version_number + 1")),
		um.Returning("id", "username", "email", "age", "created_at", "version_number"),
	)

	row, err := bob.One(ctx, w.db, query, scan.StructMapper[ProfileRow]())
	if err != nil {
		return nil, wrapProfileError(err)
	}

	prof := toProfile(row)
	return &prof, nil
}

// WithTx implements ProfileWriteStore transaction support.
func (w *PostgresProfileWriter) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, txTx domain.ProfileWriteTx) error,
) error {
	return w.txm.WithTx(ctx, func(ctx context.Context, q db.Querier) error {
		tx, ok := q.(bob.Tx)
		if !ok {
			return fmt.Errorf("querier is not a transaction")
		}

		txRepo := &profileWriterTx{
			parent: w,
			tx:     tx,
		}
		return fn(ctx, txRepo)
	})
}

// WithTimeoutTx implements ProfileWriteStore transaction support with timeout.
func (w *PostgresProfileWriter) WithTimeoutTx(
	ctx context.Context,
	timeout time.Duration,
	fn func(ctx context.Context, txTx domain.ProfileWriteTx) error,
) error {
	return w.txm.WithTimeoutTx(ctx, timeout, func(ctx context.Context, q db.Querier) error {
		tx, ok := q.(bob.Tx)
		if !ok {
			return fmt.Errorf("querier is not a transaction")
		}

		txRepo := &profileWriterTx{
			parent: w,
			tx:     tx,
		}
		return fn(ctx, txRepo)
	})
}

// profileWriterTx is a transaction-scoped writer that reuses prepared statements.
type profileWriterTx struct {
	parent *PostgresProfileWriter
	tx     bob.Tx
}

var _ domain.ProfileWriteTx = (*profileWriterTx)(nil)

func (t *profileWriterTx) CreateProfile(ctx context.Context, username, email string) (*domain.Profile, error) {
	stmt := inTxQueryStmt(ctx, t.parent.createStmt, t.tx)

	row, err := stmt.One(ctx, createProfileArgs{
		Username: username,
		Email:    email,
	})
	if err != nil {
		return nil, wrapProfileError(err)
	}

	p := toProfile(row)
	return &p, nil
}

func (t *profileWriterTx) UpdateProfile(ctx context.Context, params *domain.UpdateProfileParams) (*domain.Profile, error) {
	stmt := inTxQueryStmt(ctx, t.parent.updateStmt, t.tx)

	row, err := stmt.One(ctx, updateProfileArgs{
		ID:       params.ID,
		Username: params.Name,
		Email:    params.Email,
		Version:  params.Version,
	})
	if err != nil {
		return nil, wrapProfileError(err)
	}
	p := toProfile(row)
	return &p, nil
}

func (t *profileWriterTx) DeleteProfile(ctx context.Context, id uuid.UUID, version int64) error {
	stmt := inTxQueryStmt(ctx, t.parent.deleteStmt, t.tx)

	_, err := stmt.One(ctx, deleteProfileArgs{
		ID:      id,
		Version: version,
	})
	if err != nil {
		return wrapProfileError(err)
	}
	return nil
}

func (t *profileWriterTx) ModifyProfile(
	ctx context.Context,
	id uuid.UUID,
	version int64,
	nameSet, nameNull bool, nameVal string,
	ageSet, ageNull bool, ageVal int32,
	emailSet bool, emailVal string,
) (*domain.Profile, error) {
	if !nameSet && !ageSet && !emailSet {
		return nil, domain.ErrInvalidData
	}

	query := psql.Update(
		um.Table(t.parent.table),
		um.Where(psql.Quote("id").EQ(psql.Arg(id))),
		um.Where(psql.Quote("deleted_at").IsNull()),
		um.Where(psql.Quote("version_number").EQ(psql.Arg(version))),
	)

	// Conditionally add SET clauses
	if nameSet {
		if nameNull {
			query.Apply(um.SetCol("username").To(psql.Raw("NULL")))
		} else {
			query.Apply(um.SetCol("username").To(psql.Arg(nameVal)))
		}
	}

	if ageSet {
		if ageNull {
			query.Apply(um.SetCol("age").To(psql.Raw("NULL")))
		} else {
			query.Apply(um.SetCol("age").To(psql.Arg(ageVal)))
		}
	}

	if emailSet {
		query.Apply(um.SetCol("email").To(psql.Arg(emailVal)))
	}

	// Always increment version for optimistic locking
	query.Apply(
		um.SetCol("version_number").To(psql.Raw("version_number + 1")),
		um.Returning("id", "username", "email", "age", "created_at", "version_number"),
	)

	row, err := bob.One(ctx, t.tx, query, scan.StructMapper[ProfileRow]())
	if err != nil {
		return nil, wrapProfileError(err)
	}

	prof := toProfile(row)
	return &prof, nil
}
