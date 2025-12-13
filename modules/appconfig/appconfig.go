package appconfig

import (
	"app/modules/db/postgres"
	"app/modules/db/redis"
	"app/modules/hmac"
	"app/modules/middleware/ratelimit"
	"app/modules/telemetry"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	// TODO: on 12-factor apps on env
	Env string `env:"ENV" envDefault:"dev"`

	// --- core infra ----
	HMAC     hmac.HMACConfig         `envPrefix:"HMAC_"`
	Redis    redis.RedisConfig       `envPrefix:"REDIS_"`
	Postgres postgres.PostgresConfig `envPrefix:"POSTGRES_"`

	// --- middlewares ----
	RateLimit ratelimit.RestHTTPConfig `envPrefix:"RATE_LIMIT_"`

	// --- otel ----
	// since it has special naming conventions, we do not use prefix here
	Otel telemetry.Config
}

func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, err
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Example central validation
func validate(c *Config) error {
	// e.g. different rules per ENV
	// if c.Env == "prod" && c.HMAC.Secret == "dev-secret" { ... }
	return nil
}
