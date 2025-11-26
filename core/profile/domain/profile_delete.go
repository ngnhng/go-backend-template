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

func (app *Application) DeleteProfile(ctx context.Context, id uuid.UUID) error {
	if id.IsNil() {
		return ErrInvalidData
	}
	err := app.pool.WithTimeoutTx(ctx, 1*time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
		return app.persistence.DeleteProfile(ctx, tx, id)
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProfileNotFound
	}
	if errors.Is(err, ErrInvalidData) {
		return ErrInvalidData
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return ErrUnhandled
}
