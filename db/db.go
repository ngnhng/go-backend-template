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

package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type (

	// Querier is an interface for database queries
	Querier interface {
		// Use sqlx.ExtContext so both *sqlx.DB and *sqlx.Tx conform
		// TODO: Prepare and reuse statements
		sqlx.ExtContext
	}

	// OLTP SQL compliant database connection pool
	ConnectionPool interface {
		HealthManager
		ConnectionManager
		MigrationManager
		TxManager

		// Shutdown attempts to gracefully close all underlying connections.
		Shutdown(context.Context) error
	}

	HealthManager interface {
		// TODO: It is more complicated than just a boolean.
		// We need to keep track of multiple instances/databases
		HealthCheck() error
	}

	// ConnectionManager tries to apply read-replica pattern whenever possible
	ConnectionManager interface {
		// Writer returns a writer (primary) database connection
		// from the underlying database connection pool
		//
		// Note: Use *sqlx.Conn only when we need to pin a
		// series of statements to the same physical
		// connection without starting a transaction
		Writer() *sqlx.DB

		// Reader returns a read replica database connection
		// from the underlying database connection pool
		//
		// Should fallback to a writer connection if not
		// available
		Reader() *sqlx.DB
	}

	MigrationManager interface {
		GenerateMigration() error
		MigrateUp() error
		MigrateDown() error
	}

	TxManager interface {
		WithTx(context.Context, func(context.Context, *sqlx.Tx) error) error
		WithTimeoutTx(context.Context, time.Duration, func(context.Context, *sqlx.Tx) error) error
	}

	KV interface {
		TxGet(context.Context, string) (any, error)
		TxSet(context.Context, string, any) (any, error)
	}
)
