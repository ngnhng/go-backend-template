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

type UpdateProfileParams struct {
	ID      uuid.UUID
	Name    string
	Email   string
	Version int64
}

func (app *Application) UpdateProfile(ctx context.Context, p *UpdateProfileParams) (*Profile, error) {
	if p == nil || p.ID.IsNil() || len(p.Name) == 0 || len(p.Email) == 0 {
		return nil, ErrInvalidData
	}
	var updated *Profile
	err := app.pool.WithTimeoutTx(ctx, 1*time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
		profile, err := app.persistence.UpdateProfile(ctx, tx, p)
		if err != nil {
			return err
		}
		updated = profile
		return nil
	})
	if err == nil {
		return updated, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPrecondition
	}
	if errors.Is(err, ErrDuplicateProfile) {
		return nil, ErrDuplicateProfile
	}
	if errors.Is(err, ErrInvalidData) {
		return nil, ErrInvalidData
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return nil, ErrUnhandled
}

// ModifyProfile applies a partial update: only provided fields are updated.
func (app *Application) ModifyProfile(ctx context.Context, id uuid.UUID, version int64, nameSet bool, nameNull bool, nameVal string, ageSet bool, ageNull bool, ageVal int32, emailSet bool, emailVal string) (*Profile, error) {
	if id.IsNil() {
		return nil, ErrInvalidData
	}
	if !nameSet && !ageSet && !emailSet {
		return nil, ErrInvalidData
	}
	var updated *Profile
	err := app.pool.WithTimeoutTx(ctx, 1*time.Second, func(ctx context.Context, tx *sqlx.Tx) error {
		p, err := app.persistence.ModifyProfile(ctx, tx, id, version, nameSet, nameNull, nameVal, ageSet, ageNull, ageVal, emailSet, emailVal)
		if err != nil {
			return err
		}
		updated = p
		return nil
	})
	if err == nil {
		return updated, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPrecondition
	}
	if errors.Is(err, ErrDuplicateProfile) {
		return nil, ErrDuplicateProfile
	}
	if errors.Is(err, ErrInvalidData) {
		return nil, ErrInvalidData
	}
	slog.ErrorContext(ctx, "unexpected error", slog.Any("error", err))
	return nil, ErrUnhandled
}
