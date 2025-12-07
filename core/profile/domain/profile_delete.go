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
