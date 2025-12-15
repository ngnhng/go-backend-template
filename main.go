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

//go:generate go tool oapi-codegen -config modules/oapi/stdlib/cfg.server.profile.yaml modules/oapi/openapi-profile.yaml
//go:generate go tool oapi-codegen -config modules/oapi/stdlib/cfg.server.payment.yaml modules/oapi/openapi-payment.yaml
//go:generate go tool oapi-codegen -config modules/oapi/echo/cfg.server.profile.yaml modules/oapi/openapi-profile.yaml
//go:generate go tool oapi-codegen -config modules/oapi/echo/cfg.server.payment.yaml modules/oapi/openapi-payment.yaml
package main

import (
	"context"
	"embed"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"app/modules/appconfig"
	"app/modules/clock"
	"app/modules/db/postgres"
	"app/modules/db/redis"
	"app/modules/db/redis/counter"
	hmac_sign "app/modules/hmac"
	"app/modules/middleware"
	"app/modules/middleware/ratelimit"
	rl "app/modules/ratelimit"
	"app/modules/server"
	"app/modules/services"
	"app/modules/telemetry"

	persistence "app/core/profile/adapters/persistence/pg"

	profile_http "app/core/profile/adapters/rest"
)

// OpenAPI specs for request validation at runtime
//
//go:embed modules/oapi/*.yaml
var validationSpecFS embed.FS

func main() {
	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	// cancel the context when these signals occur
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGKILL, syscall.SIGTERM, os.Interrupt)
	defer cancel()

	// manual dependency injections, imo there's no need to over-engineer with DI frameworks like Fx or Wire
	slog.SetLogLoggerLevel(slog.LevelDebug)

	clock := clock.RealClock{}

	// --- application config ----
	appConfig, err := appconfig.Load()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	// --- infrastructure ---

	connectionPool, err := postgres.New(
		ctx,
		&appConfig.Postgres,
		postgres.PostgresOptions{
			// assuming writer connection does not pass through pgBouncer,
			// so we can apply server-side prepared statements
			ReaderOptions: []postgres.PgxConfigOption{
				postgres.WithPgBouncerSimpleProtocol(),
			},
		},
	)
	if err != nil {
		slog.ErrorContext(ctx, "database error", slog.Any("error", err))
		exitCode = 1
		return
	}
	defer func() {
		if err := connectionPool.Shutdown(ctx); err != nil {
			slog.ErrorContext(ctx, "database shutdown error", slog.Any("error", err))
		}
	}()

	// Should be a separate goroutine
	if err = connectionPool.HealthCheck(); err != nil {
		slog.ErrorContext(ctx, "database health check failed", slog.Any("error", err))
		exitCode = 1
		return
	}

	signer, err := hmac_sign.NewHMACSigner([]byte(appConfig.HMAC.Secret))
	if err != nil {
		slog.ErrorContext(ctx, "hmac signer setup error", slog.Any("error", err))
		exitCode = 1
		return
	}

	// Initialize reader (uses runtime replica selection) and writer (uses prepared statements on primary)
	reader := persistence.NewPostgresProfileReader(connectionPool, "profiles")

	writer, err := persistence.NewPostgresProfileWriter(ctx, connectionPool, "profiles")
	if err != nil {
		slog.ErrorContext(ctx, "profile writer initialization error", slog.Any("error", err))
		exitCode = 1
		return
	}

	otelShutdown, err := telemetry.Init(ctx, appConfig.Otel)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry not properly configured", slog.Any("error", err))
		exitCode = 1
		return
	}
	defer func() {
		if err := otelShutdown(ctx); err != nil {
			slog.ErrorContext(ctx, "telemetry shutdown error", slog.Any("error", err))
		}
	}()

	redisClient, err := redis.NewRueidisClient(ctx, appConfig.Redis)
	if err != nil {
		slog.ErrorContext(ctx, "redis not properly setup", slog.Any("error", err))
		exitCode = 1
		return
	}

	defer redisClient.Close()

	redisCounter := counter.NewInstrumentedRedisCounterStore(redisClient, "dev")

	keyStrategies := map[ratelimit.KeyStrategyId]ratelimit.KeyFunc{
		"remote_ip": ratelimit.RemoteIpKeyFunc,
	}

	slog.Debug("app rate limit config", slog.Any("rate_limit_config", appConfig.RateLimit))

	rtp, err := ratelimit.ParsePolicy(
		rl.SlidingWindowFactory(clock, redisCounter, "dev"),
		&appConfig.RateLimit,
		// TODO: provide same gin framework version example
		func(r *http.Request) ratelimit.RouteInfo {
			id := ratelimit.Pattern(r.Pattern)
			// pattern is empty if request is not matched again a pattern
			if r.Pattern == "" {
				id = ratelimit.Pattern(r.URL.Path)
			}
			return ratelimit.RouteInfo{
				ID:     id,
				Method: r.Method,
				Path:   r.URL.Path,
			}
		},
		keyStrategies,
	)
	if err != nil {
		slog.ErrorContext(ctx, "ratelimit config not properly parsed", slog.Any("error", err))
		exitCode = 1
		return
	}

	rateLimitMiddleware := ratelimit.NewRateLimitMiddleware(rtp)

	// --- application layer ---

	profileApi := profile_http.NewProfileService(
		reader, writer, signer)

	// Initialize HTTP metrics for middleware-based instrumentation
	httpMetrics, err := telemetry.NewHTTPMetrics("profile-api")
	if err != nil {
		slog.WarnContext(ctx, "failed to initialize HTTP metrics, continuing without metrics", slog.Any("error", err))
		httpMetrics = nil
	}

	profileSvc := services.NewProfileAPIService(
		profileApi,
		validationSpecFS,
		// TODO: fail fast when file not exists
		"modules/oapi/openapi-profile.yaml",
	)

	server, err := server.New(
		"0.0.0.0", 8080,
		server.WithWriteTimeout(10*time.Second),
		server.WithServices(profileSvc),
		server.WithGlobalMiddlewares(
			middleware.Telemetry(httpMetrics),
			rateLimitMiddleware,
			profile_http.RecoverHTTPMiddleware(),
		),
	)
	if err != nil {
		slog.ErrorContext(ctx, "init server error", slog.Any("error", err))
		exitCode = 1
		return
	}

	if err := server.Run(ctx); err != nil {
		slog.ErrorContext(ctx, "running server error", slog.Any("error", err))
		exitCode = 1
		return
	}
}
