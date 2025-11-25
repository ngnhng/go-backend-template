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

package postgres

import (
	"app/db"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	_ "github.com/caarlos0/env/v11" // what we should use to parse env

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	_ "github.com/amacneil/dbmate/v2/pkg/dbmate"
	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
)

var _ db.ConnectionPool = (*PostgresConnectionPool)(nil)

type (
	PostgresConnectionPool struct {
		writer *sqlx.DB

		readers []*sqlx.DB
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
		Port         uint16 `env:"USER"     envDefault:"5432"`
		User         string `env:"PORT"     envDefault:"postgres"`
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
func (p *PostgresConnectionPool) HealthCheck() bool {
	ctx := context.Background()
	conn, err := p.writer.Connx(ctx)
	if err != nil {
		return false
	}

	// TODO: Make this query configurable
	_, err = conn.ExecContext(ctx, "SELECT 1")
	return err != nil
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
// - Health-aware selection (cooldown & circuit breakers)
// - Power of two choices
// - Retry policy
// - Read-your-write
//
// Without any profiling/edge cases to justify implementing the more complex
// choices, here we first use a simpler approach first
func (p *PostgresConnectionPool) Reader() *sqlx.DB {

	if len(p.readers) == 0 {
		return p.Writer()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.readers[rand.IntN(len(p.readers))]
}

// WithTimeoutTx implements db.ConnectionPool.
func (p *PostgresConnectionPool) WithTimeoutTx(ctx context.Context, timeout time.Duration, fn func(context.Context, *sqlx.Tx) error) error {
	ctx, stop := context.WithTimeout(ctx, timeout)
	defer stop()

	// TODO: make isolation level configurable
	tx, err := p.writer.BeginTxx(ctx, &sql.TxOptions{
		ReadOnly: false,
	})
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err = fn(ctx, tx); err != nil {
		rbErr := tx.Rollback()
		if rbErr != nil && rbErr != sql.ErrTxDone {
			return fmt.Errorf("rollback failed after error: %v: %w", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}

// WithTx implements db.ConnectionPool.
func (p *PostgresConnectionPool) WithTx(ctx context.Context, fn func(context.Context, *sqlx.Tx) error) error {
	// TODO: make isolation level configurable
	tx, err := p.writer.BeginTxx(ctx, &sql.TxOptions{
		ReadOnly: false,
	})
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err = fn(ctx, tx); err != nil {
		rbErr := tx.Rollback()
		if rbErr != nil && rbErr != sql.ErrTxDone {
			return fmt.Errorf("rollback failed after error: %v: %w", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}

// Writer implements db.ConnectionPool.
func (p *PostgresConnectionPool) Writer() *sqlx.DB {
	return p.writer
}

// Example:
// postgres://jack:secret@pg.example.com:5432/mydb?sslmode=verify-ca&pool_max_conns=10&pool_max_conn_lifetime=1h30m
func connString(cfg *PoolConfig) string {
	slog.Debug("config debug", slog.Any("postgres url", fmt.Sprintf("%+v", cfg)))
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?pool_max_conns=%v", cfg.User, cfg.Password, cfg.Host, strconv.Itoa(int(cfg.Port)), cfg.Database, cfg.PoolMaxConns)
}

func New(ctx context.Context, config *PostgresConnectionConfig) (*PostgresConnectionPool, error) {
	writer, err := initDBFromConfig(ctx, &config.WriteConfig)
	if err != nil {
		return nil, err
	}

	var readers []*sqlx.DB
	for _, r := range config.ReadConfigs {
		reader, err := initDBFromConfig(ctx, &r)
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

func initDBFromConfig(ctx context.Context, config *PoolConfig) (*sqlx.DB, error) {
	poolConfig, err := pgxpool.ParseConfig(connString(config))
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	return sqlx.NewDb(stdlib.OpenDBFromPool(pool), "pgx"), nil
}
