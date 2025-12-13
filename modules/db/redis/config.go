package redis

import "time"

// TODO: CAS/WATCH based flows
// TODO: sentinel

// RedisConfig contains configuration for constructing a rueidis.Client.
//
// URL is a standard Redis URI, for example:
//
//   - Single:  redis://:password@localhost:6379/0
//   - TLS:     rediss://:password@my-redis.example.com:6379/0
//   - Cluster: redis://:password@host1:6379/0?addr=host2:6379&addr=host3:6379
//
// Cluster vs single vs sentinel is auto-detected by rueidis based on InitAddress and options.
type RedisConfig struct {
	// Required: Redis connection URL (redis:// or rediss://).
	URL string `env:"URL" envDefault:"redis://:redis@localhost:6379/0"`

	// Optional: client name visible in CLIENT LIST, etc.
	ClientName string `env:"CLIENT_NAME"`

	// SkipTLSVerify disables TLS certificate verification. Only use this in trusted
	// environments (e.g. some AWS ElastiCache setups with non-standard certs).
	SkipTLSVerify bool `env:"SKIP_TLS_VERIFY"`

	// AutoDetectAWS enables AWS-specific heuristics (e.g. ElastiCache endpoints
	// with non-standard certificates). When true, SkipTLSVerify will be turned on
	// automatically for *.cache.amazonaws.com URLs. This is off by default to
	// avoid surprising TLS downgrades.
	AutoDetectAWS bool `env:"AUTO_DETECT_AWS"`

	// RequireTLS enforces the use of rediss:// (or other TLS-enabled schemes).
	// If true and the URL is redis://, NewRueidisClient returns an error; if
	// false, we log a warning when TLS-related options are set on redis://.
	RequireTLS bool `env:"REQUIRE_TLS"`

	// Tuning flags — leave zero-valued to keep rueidis defaults.
	DisableRetry      bool          `env:"DISABLE_RETRY"`
	DisableCache      bool          `env:"DISABLE_CACHE"`
	AlwaysPipelining  bool          `env:"ALWAYS_PIPELINING"`
	ConnWriteTimeout  time.Duration `env:"CONN_WRITE_TIMEOUT"`
	RingScaleEachConn int           `env:"RING_SCALE_EACH_CONN"`
	CacheSizeEachConn int           `env:"CACHE_SIZE_EACH_CONN"`

	// Enable OpenTelemetry integration via rueidisotel.WithClient.
	EnableOtel bool `env:"ENABLE_OTEL"`

	// Enable server-assisted client-side caching for the given prefixes.
	// Example: []string{"app:profile:", "app:session:"}
	//
	// NOTE: this just configures CLIENT TRACKING ON with PREFIX/BCAST/OPTIN.
	// You still opt-in per-command using DoCache() on the client.
	ClientTrackingPrefixes []string `env:"CLIENT_TRACKING_PREFIXES" envSeparator:","`
}
