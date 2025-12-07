// Copyright 2025 Nhat-Nguyen Nguyen
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
