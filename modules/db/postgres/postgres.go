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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"app/modules/db"

	_ "github.com/caarlos0/env/v11" // what we should use to parse env

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stephenafamo/bob"

	_ "github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
)

var _ db.ConnectionPool = (*PostgresConnectionPool)(nil)

type (
	PostgresConnectionPool struct {
		writer bob.DB

		readers []bob.DB
		mu      sync.Mutex

		// TODO: partitioning config
	}

	// Note: For env parsing to work, we must export all struct fields
	PostgresConnectionConfig struct {
		WriteConfig PoolConfig   `envPrefix:"POSTGRES_PRIMARY_"`
		ReadConfigs []PoolConfig `envPrefix:"POSTGRES_REPLICA_"`
	}

	PoolConfig struct {
		Host         string `env:"HOST"     envDefault:"localhost"`
		Port         uint16 `env:"PORT"     envDefault:"5432"`
		User         string `env:"USER"     envDefault:"postgres"`
		Password     string `env:"PASSWORD" envDefault:"postgres"`
		Database     string `env:"DATABASE" envDefault:"postgres"`
		PoolMaxConns int    `env:"POOL_MAX_CONNS" envDefault:"5"`
	}
)

// GenerateMigration implements db.ConnectionPool.
func (p *PostgresConnectionPool) GenerateMigration() error {
	panic("unimplemented")
}

// HealthCheck implements db.ConnectionPool.
func (p *PostgresConnectionPool) HealthCheck() error {
	ctx := context.Background()
	// TODO: Make this query configurable
	_, err := p.writer.ExecContext(ctx, "SELECT 1")
	return err
}

// MigrateDown implements db.ConnectionPool.
func (p *PostgresConnectionPool) MigrateDown() error {
	panic("unimplemented")
}

// MigrateUp implements db.ConnectionPool.
func (p *PostgresConnectionPool) MigrateUp() error {
	panic("unimplemented")
}

// Reader implements db.ConnectionPool.
//
// Many strategies exist for selecting one reader from the list:
// - Health-aware selection (cool-down & circuit breakers)
// - Power of two choices
// - Retry policy
// - Read-your-write
//
// Without any profiling/edge cases to justify implementing the more complex
// choices, here we first use a simpler approach first
func (p *PostgresConnectionPool) Reader() db.Querier {
	if len(p.readers) == 0 {
		return p.Writer()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.readers[rand.IntN(len(p.readers))]
}

// WithTimeoutTx implements db.ConnectionPool.
func (p *PostgresConnectionPool) WithTimeoutTx(ctx context.Context, timeout time.Duration, fn db.TxFn) error {
	ctx, stop := context.WithTimeout(ctx, timeout)
	defer stop()

	return p.WithTx(ctx, fn)
}

// WithTx implements db.ConnectionPool.
func (p *PostgresConnectionPool) WithTx(ctx context.Context, fn db.TxFn) error {
	// TODO: make isolation level configurable
	return p.writer.RunInTx(ctx, &sql.TxOptions{
		ReadOnly: false,
	}, func(ctx context.Context, exec bob.Executor) error {
		// exec implements bob.Executor, which satisfies our db.Querier
		return fn(ctx, exec)
	})
}

// Shutdown implements db.ConnectionPool.
func (p *PostgresConnectionPool) Shutdown(_ context.Context) error {
	if p == nil {
		return nil
	}

	var errs []error

	if err := p.writer.Close(); err != nil {
		errs = append(errs, err)
	}

	for _, reader := range p.readers {
		if err := reader.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Using errors.Join() can blow up memory if we have big nested structures (https://github.com/golangci/golangci-lint/issues/5883)
	// Don't:
	// 		for _, e := range someErrors {
	// 			err = errors.Join(err, e) // join previous joined error again
	// 		}
	//
	// Do:
	//     err = errors.Join(errs...) // single, flat join
	return errors.Join(errs...)
}

// Writer implements db.ConnectionPool.
func (p *PostgresConnectionPool) Writer() db.Querier {
	return p.writer
}

// Primary returns the primary (writer) bob.DB instance.
// This is used for preparing write statements.
func (p *PostgresConnectionPool) Primary() *bob.DB {
	return &p.writer
}

// Replica returns a random replica bob.DB instance, or the primary if no replicas exist.
// This is used for preparing read statements.
func (p *PostgresConnectionPool) Replica() *bob.DB {
	if len(p.readers) == 0 {
		return &p.writer
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return &p.readers[rand.IntN(len(p.readers))]
}

// Example:
// postgres://jack:secret@pg.example.com:5432/mydb?sslmode=verify-ca&pool_max_conns=10&pool_max_conn_lifetime=1h30m
func connString(cfg *PoolConfig) string {
	slog.Debug("config debug", slog.Any("postgres url", fmt.Sprintf("%+v", cfg)))
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?pool_max_conns=%v", cfg.User, cfg.Password, cfg.Host, strconv.Itoa(int(cfg.Port)), cfg.Database, cfg.PoolMaxConns)
}

func New(
	ctx context.Context,
	config *PostgresConnectionConfig,
	opts PostgresOptions,
) (*PostgresConnectionPool, error) {
	writer, err := initDBFromConfig(ctx, &config.WriteConfig, opts.WriterOptions...)
	if err != nil {
		return nil, err
	}

	var readers []bob.DB
	for _, r := range config.ReadConfigs {
		reader, err := initDBFromConfig(ctx, &r, opts.ReaderOptions...)
		if err != nil {
			// TODO: continue or abort?
			return nil, err
		}
		readers = append(readers, reader)
	}

	return &PostgresConnectionPool{
		writer:  writer,
		readers: readers,
	}, nil
}

func initDBFromConfig(
	ctx context.Context,
	config *PoolConfig,
	opts ...PgxConfigOption,
) (bob.DB, error) {
	poolConfig, err := pgxpool.ParseConfig(connString(config))
	if err != nil {
		return bob.DB{}, err
	}

	for _, opt := range opts {
		if opt != nil {
			opt(poolConfig)
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return bob.DB{}, err
	}
	return bob.NewDB(stdlib.OpenDBFromPool(pool)), nil
}
