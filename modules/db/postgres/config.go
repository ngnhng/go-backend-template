package postgres

type (
	// Note: For env parsing to work, we must export all struct fields
	PostgresConfig struct {
		WriteConfig PoolConfig   `envPrefix:"PRIMARY_"`
		ReadConfigs []PoolConfig `envPrefix:"REPLICA_"`
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
