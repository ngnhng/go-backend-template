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
	"app/db/postgres"
	"app/server"
	"app/services"
	"app/telemetry"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"

	hmacsign "app/hmac"
	profile_service "app/profile-service"
)

func main() {
	// cancel the context when these signals occur
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGKILL, syscall.SIGTERM, os.Interrupt)
	defer cancel()

	// manual dependency injections, imo there's no need to over-engineer with DI frameworks like Fx or Wire
	slog.SetLogLoggerLevel(slog.LevelDebug)

	// --- infrastructure ---

	postgresDBConfig, err := env.ParseAs[postgres.PostgresConnectionConfig]()
	if err != nil {
		slog.ErrorContext(ctx, "config error", slog.Any("error", err))
		os.Exit(1)
	}
	postgresConnectionPool, err := postgres.New(ctx, &postgresDBConfig)
	if err != nil {
		slog.ErrorContext(ctx, "database error", slog.Any("error", err))
		os.Exit(1)
	}
	if !postgresConnectionPool.HealthCheck() {
		slog.ErrorContext(ctx, "database health check failed")
		os.Exit(1)
	}

	hmacConfig, err := env.ParseAs[hmacsign.HMACConfig]()
	if err != nil {
		slog.ErrorContext(ctx, "hmac key not configured")
		os.Exit(1)
	}
	signer, err := hmacsign.NewHMACSigner([]byte(hmacConfig.Secret))
	if err != nil {
		slog.ErrorContext(ctx, "hmac signer setup error", slog.Any("error", err))
		os.Exit(1)
	}

	postgresProfilePersistence := &profile_service.PostgresProfilePersistence{
		TableName: "profiles",
	}

	telemetryConfig, err := env.ParseAs[telemetry.Config]()
	if err != nil {
		slog.ErrorContext(ctx, "telemetry not properly configured")
		os.Exit(1)
	}
	otelShutdown, err := telemetry.Init(ctx, telemetryConfig)
	if err != nil {
		slog.ErrorContext(ctx, "telemetry not properly configured")
		os.Exit(1)
	}
	defer otelShutdown(ctx)

	// --- application layer ---

	profileApi := profile_service.NewProfileService(
		postgresConnectionPool, postgresProfilePersistence, signer)

	// Compose enabled services
	profileSvc := services.NewProfileAPIService(profileApi)

	server, err := server.New(
		"0.0.0.0", 8080,
		server.WithWriteTimeout(10*time.Second),
		server.WithServices(profileSvc),
		server.WithGlobalMiddlewares(profile_service.RecoverHTTPMiddleware()),
	)
	if err != nil {
		slog.ErrorContext(ctx, "init server error", slog.Any("error", err))
		os.Exit(1)
	}

	if err := server.Run(ctx); err != nil {
		slog.ErrorContext(ctx, "running server error", slog.Any("error", err))
		os.Exit(1)
	}
	os.Exit(0)
}
