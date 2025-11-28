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

//go:generate go tool oapi-codegen -config oapi/stdlib/cfg.server.profile.yaml oapi/profile-api-spec.yaml
//go:generate go tool oapi-codegen -config oapi/stdlib/cfg.server.payment.yaml oapi/payment-api-spec.yaml
//go:generate go tool oapi-codegen -config oapi/echo/cfg.server.profile.yaml oapi/profile-api-spec.yaml
//go:generate go tool oapi-codegen -config oapi/echo/cfg.server.payment.yaml oapi/payment-api-spec.yaml
package main

import (
	"context"
	"embed"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"app/db/postgres"
	"app/middleware"
	"app/server"
	"app/services"
	"app/telemetry"

	"github.com/caarlos0/env/v11"

	"app/core/profile/adapters/persistence"

	profile_http "app/core/profile/adapters/http"
	hmac_sign "app/hmac"
)

// OpenAPI specs for request validation at runtime
//
//go:embed oapi/*.yaml
var specFS embed.FS

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

	// --- infrastructure ---

	postgresDBConfig, err := env.ParseAs[postgres.PostgresConnectionConfig]()
	if err != nil {
		slog.ErrorContext(ctx, "config error", slog.Any("error", err))
		exitCode = 1
		return
	}
	postgresConnectionPool, err := postgres.New(ctx, &postgresDBConfig)
	if err != nil {
		slog.ErrorContext(ctx, "database error", slog.Any("error", err))
		exitCode = 1
		return
	}
	defer func() {
		if err := postgresConnectionPool.Shutdown(ctx); err != nil {
			slog.ErrorContext(ctx, "database shutdown error", slog.Any("error", err))
		}
	}()

	if err = postgresConnectionPool.HealthCheck(); err != nil {
		slog.ErrorContext(ctx, "database health check failed", slog.Any("error", err))
		exitCode = 1
		return
	}

	hmacConfig, err := env.ParseAs[hmac_sign.HMACConfig]()
	if err != nil {
		slog.ErrorContext(ctx, "hmac key not configured", slog.Any("error", err))
		exitCode = 1
		return
	}
	signer, err := hmac_sign.NewHMACSigner([]byte(hmacConfig.Secret))
	if err != nil {
		slog.ErrorContext(ctx, "hmac signer setup error", slog.Any("error", err))
		exitCode = 1
		return
	}

	postgresProfilePersistence := &persistence.PostgresProfilePersistence{
		TableName: "profiles",
	}

	telemetryConfig, err := env.ParseAs[telemetry.Config]()
	if err != nil {
		slog.ErrorContext(ctx, "telemetry not properly configured", slog.Any("error", err))
		exitCode = 1
		return
	}
	otelShutdown, err := telemetry.Init(ctx, telemetryConfig)
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

	// --- application layer ---

	profileApi := profile_http.NewProfileService(
		postgresConnectionPool, postgresProfilePersistence, signer)

	// Initialize HTTP metrics for middleware-based instrumentation
	httpMetrics, err := telemetry.NewHTTPMetrics("profile-api")
	if err != nil {
		slog.WarnContext(ctx, "failed to initialize HTTP metrics, continuing without metrics", slog.Any("error", err))
		httpMetrics = nil
	}

	profileSvc := services.NewProfileAPIService(
		profileApi,
		specFS,
		"oapi/profile-api-spec.yaml",
	)

	server, err := server.New(
		"0.0.0.0", 8080,
		server.WithWriteTimeout(10*time.Second),
		server.WithServices(profileSvc),
		server.WithGlobalMiddlewares(
			middleware.Telemetry(httpMetrics),
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
