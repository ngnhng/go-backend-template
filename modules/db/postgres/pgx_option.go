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

package postgres

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgxConfigOption func(cfg *pgxpool.Config)

type PostgresOptions struct {
	WriterOptions []PgxConfigOption
	ReaderOptions []PgxConfigOption
}

// WithPgBouncerSimpleProtocol configures pgx for PgBouncer (transaction pooling).
// This disables server-side prepared statements for PgBouncer compatibility.
//
// PgBouncer in transaction pooling mode does NOT support prepared statements
// because each query might go to a different backend connection.
func WithPgBouncerSimpleProtocol() PgxConfigOption {
	return func(cfg *pgxpool.Config) {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
}
