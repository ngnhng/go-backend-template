package domain

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

func (app *Application) DeleteProfile(ctx context.Context, id uuid.UUID, version int64) error {
	if id.IsNil() || version < 0 {
		return ErrInvalidData
	}
	err := app.pool.WithTimeoutTx(ctx, 1*time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
		return app.persistence.DeleteProfile(ctx, tx, id, version)
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPrecondition
	}
	if errors.Is(err, ErrInvalidData) {
		return ErrInvalidData
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return ErrUnhandled
}
