package domain

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gofrs/uuid/v5"
)

func (app *Application) DeleteProfile(ctx context.Context, id uuid.UUID, version int64) error {
	if id.IsNil() || version < 0 {
		return ErrInvalidData
	}
	err := app.writer.WithTx(ctx, func(ctx context.Context, tx ProfileWriteTx) error {
		return tx.DeleteProfile(ctx, id, version)
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrProfileNotFound) {
		return ErrPrecondition
	}
	if errors.Is(err, ErrInvalidData) {
		return ErrInvalidData
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return ErrUnhandled
}
