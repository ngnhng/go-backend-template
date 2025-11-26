package profile_service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
)

func (app *Application) CreateProfile(ctx context.Context, username, email string) (*Profile, error) {
	if len(username) == 0 {
		slog.ErrorContext(ctx, "invalid name", slog.Any("name", username))
		return nil, ErrInvalidData
	}
	var created *Profile
	err := app.pool.WithTimeoutTx(ctx, 1*time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
		p, err := app.persistence.CreateProfile(ctx, tx, username, email)
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
	if errors.Is(err, ErrDuplicateEntry) {
		slog.ErrorContext(ctx, "duplicate entry", slog.Any("name", username))
		return nil, ErrDuplicateProfile
	}

	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return nil, ErrUnhandled
}
