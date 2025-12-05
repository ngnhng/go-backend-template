package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

func (app *Application) CreateProfile(ctx context.Context, username, email string) (*Profile, error) {
	if len(username) == 0 {
		slog.ErrorContext(ctx, "invalid name", slog.Any("name", username))
		return nil, ErrInvalidData
	}
	var created *Profile
	err := app.writer.WithTx(ctx, func(ctx context.Context, tx ProfileWriteTx) error {
		p, err := tx.CreateProfile(ctx, username, email)
		if err != nil {
			return err
		}
		created = p
		return nil
	})
	if err == nil {
		slog.DebugContext(ctx, "created profile", slog.Any("profile", fmt.Sprintf("%+v", created)))
		return created, nil
	}
	if errors.Is(err, ErrDuplicateProfile) {
		slog.ErrorContext(ctx, "duplicate entry", slog.Any("name", username))
		return nil, ErrDuplicateProfile
	}

	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return nil, ErrUnhandled
}
