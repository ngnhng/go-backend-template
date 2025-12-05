package domain

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gofrs/uuid/v5"
)

func (app *Application) GetProfileByID(ctx context.Context, id uuid.UUID) (*Profile, error) {
	if id.IsNil() {
		return nil, ErrInvalidData
	}
	prof, err := app.reader.GetProfileByID(ctx, id)
	if err == nil {
		return prof, nil
	}
	if errors.Is(err, ErrProfileNotFound) {
		return nil, ErrProfileNotFound
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return nil, ErrUnhandled
}
