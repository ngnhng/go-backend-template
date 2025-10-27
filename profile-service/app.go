// Copyright 2025 Nguyen Nhat Nguyen
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

package profile_service

import (
	"app/db"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

var (
	ErrDuplicateProfile = errors.New("profile with the requested identifiers already exists")
	ErrInvalidData      = errors.New("invalid data provided for profile operations")
	ErrUnhandled        = errors.New("unexected error")
)

type (
	ProfileManager struct {
		pool        db.ConnectionPool
		persistence ProfilePersistence
		signer      CursorSigner
	}

	// Note: We pass db.Querier for read/write routing at the application layer
	ProfilePersistence interface {
		CreateProfile(context.Context, db.Querier, string, string) (*Profile, error)
		GetProfilesByOffset(context.Context, db.Querier, int, int) ([]Profile, int, error)
		GetProfilesByCursor(context.Context, db.Querier, time.Time, uuid.UUID, CursorDirection, int) ([]Profile, error)
		GetProfilesFirstPage(context.Context, db.Querier, int) ([]Profile, error)
	}

	CursorSigner interface {
		// Sign returns token = base64url(payload) + "." + base64url(algo(payloadB64))
		Sign(payload []byte) (string, error)
		// Verify returns the original payload after validating HMAC; error if invalid
		Verify(token string) ([]byte, error)
	}

	// Dual use as domain model and DB Entity for now
	Profile struct {
		ID        uuid.UUID     `db:"id"`
		Name      string        `db:"username"`
		Email     string        `db:"email"`
		Age       sql.NullInt32 `db:"age"`
		CreatedAt time.Time     `db:"created_at"`
	}
)

func newApp(pool db.ConnectionPool, persistence ProfilePersistence, signer CursorSigner) *ProfileManager {
	return &ProfileManager{pool: pool, persistence: persistence, signer: signer}
}

func (app *ProfileManager) CreateProfile(ctx context.Context, username, email string) (*Profile, error) {
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

func (app *ProfileManager) GetProfilesByOffset(ctx context.Context, page int, pageSize int) ([]Profile, int, error) {
	if page < 0 || pageSize <= 0 {
		return nil, 0, ErrInvalidData
	}
	offset := page * pageSize
	profiles, count, err := app.persistence.GetProfilesByOffset(ctx, app.pool.Reader(), pageSize, offset)
	if err != nil {
		slog.ErrorContext(ctx, "persistence error", slog.Any("error", err))
		return nil, 0, err
	}
	return profiles, count, nil
}

const (
	ASC  CursorDirection = "asc"
	DESC CursorDirection = "desc"
)

type (
	CursorDirection string

	CursorPaginationToken struct {
		TTL       time.Time       `json:"ttl"`
		Direction CursorDirection `json:"direction"`

		Pivot struct {
			CreatedAt time.Time `json:"created_at"`
			ID        uuid.UUID `json:"id"`
		} `json:"pivot"`

		Signature string `json:"-"`
	}
)

func (app *ProfileManager) GetProfilesByCursor(ctx context.Context, rawCursor string, limit int) ([]Profile, string, error) {
	if limit <= 0 {
		return nil, "", ErrInvalidData
	}

	tok, err := app.decodeCursorToken(rawCursor)
	if err != nil {
		slog.ErrorContext(ctx, "invalid cursor", slog.Any("error", err))
		return nil, "", ErrInvalidData
	}

	profiles, err := app.persistence.GetProfilesByCursor(ctx, app.pool.Reader(), tok.Pivot.CreatedAt, tok.Pivot.ID, tok.Direction, limit)
	if err != nil {
		slog.ErrorContext(ctx, "persistence error", slog.Any("error", err))
		return nil, "", err
	}
	// next/prev cursors are derived at API layer; keep return shape
	return profiles, "", nil
}

// --- cursor helpers (opaque token: base64url(JSON) . base64url(HMAC)) ---

func (app *ProfileManager) encodeCursorToken(tok *CursorPaginationToken) (string, error) {
	if tok == nil {
		return "", ErrInvalidData
	}
	if app.signer == nil {
		return "", ErrInvalidData
	}
	b, err := json.Marshal(tok)
	if err != nil {
		return "", err
	}
	return app.signer.Sign(b)
}

func (app *ProfileManager) decodeCursorToken(s string) (*CursorPaginationToken, error) {
	if s == "" {
		return nil, ErrInvalidData
	}
	if app.signer == nil {
		return nil, ErrInvalidData
	}
	raw, err := app.signer.Verify(s)
	if err != nil {
		return nil, ErrInvalidData
	}
	var tok CursorPaginationToken
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, ErrInvalidData
	}
	if tok.TTL.IsZero() || time.Now().After(tok.TTL) {
		return nil, ErrInvalidData
	}
	if tok.Direction != ASC && tok.Direction != DESC {
		return nil, ErrInvalidData
	}
	return &tok, nil
}

func (app *ProfileManager) makeCursorFromProfile(p Profile, dir CursorDirection, ttl time.Duration) string {
	tok := &CursorPaginationToken{
		TTL:       time.Now().Add(ttl),
		Direction: dir,
	}
	tok.Pivot.CreatedAt = p.CreatedAt
	tok.Pivot.ID = p.ID
	s, err := app.encodeCursorToken(tok)
	if err != nil {
		return ""
	}
	return s
}

// First page for cursor mode (no client-provided cursor)
func (app *ProfileManager) GetProfilesFirstPage(ctx context.Context, limit int) ([]Profile, error) {
	if limit <= 0 {
		return nil, ErrInvalidData
	}
	return app.persistence.GetProfilesFirstPage(ctx, app.pool.Reader(), limit)
}
