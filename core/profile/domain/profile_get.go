package domain

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/gofrs/uuid/v5"
)

func (app *Application) GetProfileByID(ctx context.Context, id uuid.UUID) (*Profile, error) {
	if id.IsNil() {
		return nil, ErrInvalidData
	}
	prof, err := app.persistence.GetProfileByID(ctx, app.pool.Reader(), id)
	if err == nil {
		return prof, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProfileNotFound
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return nil, ErrUnhandled
}
