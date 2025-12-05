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
