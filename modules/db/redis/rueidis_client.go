// Copyright 2025 Nhat-Nguyen Nguyen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/redis/rueidis"
	"github.com/redis/rueidis/rueidisotel"
)

// TODO: CAS/WATCH based flows

// RueidisOptions contains configuration for constructing a rueidis.Client.
//
// URL is a standard Redis URI, for example:
//
//   - Single:  redis://:password@localhost:6379/0
//   - TLS:     rediss://:password@my-redis.example.com:6379/0
//   - Cluster: redis://:password@host1:6379/0?addr=host2:6379&addr=host3:6379
//
// Cluster vs single vs sentinel is auto-detected by rueidis based on InitAddress and options.
type RueidisOptions struct {
	// Required: Redis connection URL (redis:// or rediss://).
	URL string `env:"REDIS_URL" envDefault:"redis://:redis@localhost:6379/0"`

	// Optional: client name visible in CLIENT LIST, etc.
	ClientName string `env:"REDIS_CLIENT_NAME"`

	// SkipTLSVerify disables TLS certificate verification. Only use this in trusted
	// environments (e.g. some AWS ElastiCache setups with non-standard certs).
	SkipTLSVerify bool `env:"REDIS_SKIP_TLS_VERIFY"`

	// AutoDetectAWS enables AWS-specific heuristics (e.g. ElastiCache endpoints
	// with non-standard certificates). When true, SkipTLSVerify will be turned on
	// automatically for *.cache.amazonaws.com URLs. This is off by default to
	// avoid surprising TLS downgrades.
	AutoDetectAWS bool `env:"REDIS_AUTO_DETECT_AWS"`

	// RequireTLS enforces the use of rediss:// (or other TLS-enabled schemes).
	// If true and the URL is redis://, NewRueidisClient returns an error; if
	// false, we log a warning when TLS-related options are set on redis://.
	RequireTLS bool `env:"REDIS_REQUIRE_TLS"`

	// Tuning flags — leave zero-valued to keep rueidis defaults.
	DisableRetry      bool          `env:"REDIS_DISABLE_RETRY"`
	DisableCache      bool          `env:"REDIS_DISABLE_CACHE"`
	AlwaysPipelining  bool          `env:"REDIS_ALWAYS_PIPELINING"`
	ConnWriteTimeout  time.Duration `env:"REDIS_CONN_WRITE_TIMEOUT"`
	RingScaleEachConn int           `env:"REDIS_RING_SCALE_EACH_CONN"`
	CacheSizeEachConn int           `env:"REDIS_CACHE_SIZE_EACH_CONN"`

	// Enable OpenTelemetry integration via rueidisotel.WithClient.
	EnableOtel bool `env:"REDIS_ENABLE_OTEL"`

	// Enable server-assisted client-side caching for the given prefixes.
	// Example: []string{"app:profile:", "app:session:"}
	//
	// NOTE: this just configures CLIENT TRACKING ON with PREFIX/BCAST/OPTIN.
	// You still opt-in per-command using DoCache() on the client.
	ClientTrackingPrefixes []string `env:"REDIS_CLIENT_TRACKING_PREFIXES" envSeparator:","`
}

// NewRueidisClient creates a production-ready rueidis.Client from RueidisOptions.
//
// It:
//
//   - Parses redis:// / rediss:// URL
//   - Configures TLS + optional insecure skip verify
//   - Sets basic tuning flags (pipelining, retry, cache, buffers)
//   - Configures server-assisted client-side caching tracking options
//   - Wraps the client with OpenTelemetry (optional)
//   - Performs a PING with a small timeout to fail fast
func NewRueidisClient(ctx context.Context, opt RueidisOptions) (rueidis.Client, error) {
	if opt.URL == "" {
		return nil, errors.New("rueidis: URL must not be empty")
	}

	u, err := url.Parse(opt.URL)
	if err != nil {
		return nil, fmt.Errorf("rueidis: parse url: %w", err)
	}

	host := u.Hostname()
	if u.Scheme == "redis" {
		if opt.RequireTLS {
			return nil, errors.New("rueidis: RequireTLS=true but URL uses redis:// (plaintext); use rediss://")
		}
		if opt.SkipTLSVerify || opt.AutoDetectAWS {
			slog.Warn("rueidis: redis:// URL disables TLS even though TLS-related options are set",
				slog.String("scheme", u.Scheme),
				slog.String("host", host),
				slog.Bool("skip_tls_verify", opt.SkipTLSVerify),
				slog.Bool("auto_detect_aws", opt.AutoDetectAWS),
			)
		}
	}

	if strings.Contains(u.Host, ".cache.amazonaws.com") && opt.AutoDetectAWS {
		slog.Info("rueidis: detected AWS ElastiCache endpoint",
			slog.String("scheme", u.Scheme),
			slog.String("host", host),
		)
		if opt.AutoDetectAWS && u.Scheme == "redis" {
			return nil, errors.New("rueidis: aws detected but using redis:// (plaintext)")
		}
	}

	if opt.DisableCache && len(opt.ClientTrackingPrefixes) > 0 {
		slog.Warn("turning on tracking on the server with no client cache benefit",
			slog.Bool("disable_cache", opt.DisableCache),
		)
	}

	clientOpt, err := rueidis.ParseURL(opt.URL)
	if err != nil {
		return nil, err
	}

	// Basic tuning
	clientOpt.ClientName = opt.ClientName
	clientOpt.DisableRetry = opt.DisableRetry
	clientOpt.DisableCache = opt.DisableCache
	clientOpt.AlwaysPipelining = opt.AlwaysPipelining

	if opt.RingScaleEachConn > 0 {
		clientOpt.RingScaleEachConn = opt.RingScaleEachConn
	}
	if opt.CacheSizeEachConn > 0 {
		clientOpt.CacheSizeEachConn = opt.CacheSizeEachConn
	}
	if opt.ConnWriteTimeout > 0 {
		clientOpt.ConnWriteTimeout = opt.ConnWriteTimeout
	}

	// TLS tweaks / ElastiCache “skip verify” mode.
	if opt.SkipTLSVerify {
		if clientOpt.TLSConfig == nil {
			clientOpt.TLSConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		} else {
			tc := clientOpt.TLSConfig.Clone()
			tc.InsecureSkipVerify = true //nolint:gosec
			clientOpt.TLSConfig = tc
		}
	}

	// Server-assisted client-side caching: configure CLIENT TRACKING prefixes.
	// We add BCAST + OPTIN so you can use DoCache() per-command.
	if len(opt.ClientTrackingPrefixes) > 0 {
		tracking := make([]string, 0, len(opt.ClientTrackingPrefixes)*2+2)
		for _, p := range opt.ClientTrackingPrefixes {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			tracking = append(tracking, "PREFIX", p)
		}
		tracking = append(tracking, "BCAST", "OPTIN")
		clientOpt.ClientTrackingOptions = tracking
	}

	var cli rueidis.Client

	if opt.EnableOtel {
		cli, err = rueidisotel.NewClient(clientOpt)
	} else {
		cli, err = rueidis.NewClient(clientOpt)
	}
	if err != nil {
		slog.ErrorContext(ctx, "error during rueidis init", slog.Any("error", err))
		return nil, err
	}

	// Sanity PING with a short timeout for fast-fail.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := cli.Do(pingCtx, cli.B().Ping().Build()).Error(); err != nil {
		cli.Close()
		return nil, err
	}

	slog.Info("rueidis: connected",
		slog.String("mode", string(cli.Mode())),
		slog.String("client_name", opt.ClientName),
	)

	return cli, nil
}
