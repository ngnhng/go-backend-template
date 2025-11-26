package domain

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

func (app *Application) GetProfilesByOffset(ctx context.Context, page int, pageSize int) ([]Profile, int, error) {
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

func (app *Application) GetProfilesByCursor(ctx context.Context, rawCursor string, limit int) ([]Profile, string, error) {
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

func (app *Application) encodeCursorToken(tok *CursorPaginationToken) (string, error) {
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

func (app *Application) decodeCursorToken(s string) (*CursorPaginationToken, error) {
	if s == "" {
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

func (app *Application) MakeCursorFromProfile(p Profile, dir CursorDirection, ttl time.Duration) string {
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
func (app *Application) GetProfilesFirstPage(ctx context.Context, limit int) ([]Profile, error) {
	if limit <= 0 {
		return nil, ErrInvalidData
	}
	return app.persistence.GetProfilesFirstPage(ctx, app.pool.Reader(), limit)
}
