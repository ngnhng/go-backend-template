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

package db

import (
	"context"
	"time"

	"github.com/stephenafamo/bob"
)

type (
	TxFn func(ctx context.Context, q Querier) error

	// Querier is an interface for database queries
	// Uses bob.Executor so both bob.DB and bob.Tx conform
	//
	// TODO: consider for removal
	Querier interface {
		bob.Executor
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
		// TODO: multiple writers
		Writer() Querier

		ReaderConnectionManager
	}

	ReaderConnectionManager interface {
		// Reader returns a read replica database connection
		// from the underlying database connection pool
		//
		// Should fallback to a writer connection if not
		// available
		Reader() Querier
	}

	MigrationManager interface {
		GenerateMigration() error
		MigrateUp() error
		MigrateDown() error
	}

	TxManager interface {
		WithTx(ctx context.Context, fn TxFn) error
		WithTimeoutTx(ctx context.Context, timeout time.Duration, fn TxFn) error
	}

	// TODO: abstract Transformer logic with generics (KVTransformer)
	//   func Getx[Tr Transformer[K, V], K, V any]
	KV interface {
		AtomicGet(context.Context, string) (any, error)
		AtomicSet(context.Context, string, any) (any, error)
	}
)
