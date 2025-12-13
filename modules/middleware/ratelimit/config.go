package ratelimit

import (
	"time"
)

type KeyStrategyId string

const (
	RemoteIpKeyStrategy KeyStrategyId = "remote_ip"
)

// TODO: sane defaults so the apps run right out of the box
type (
	RestHTTPConfig struct {
		Routes              []Route      `envPrefix:"ROUTE_"`
		DefaultPolicy       EndpointRule `envPrefix:"DEFAULT_"`
		AllowIfNoMatch      bool         `env:"ALLOW_IF_NO_MATCH"`
		AllowIfNoIdentifier bool         `env:"ALLOW_IF_NO_ID"`
	}

	Route struct {
		// TODO: define common convention for pattern
		Pattern       string         `env:"PATTERN"`
		EndpointRules []EndpointRule `envPrefix:"POLICY_"`
	}

	EndpointRule struct {
		Method      string        `env:"METHOD"`
		Limit       int64         `env:"LIMIT" envDefault:"10000"`
		Window      time.Duration `env:"WINDOW"`
		KeyStrategy KeyStrategyId `env:"KEY_STRATEGY"`
	}
)
