# API Service Template

This repository is a pragmatic, production-lean template for building HTTP API services in Go. It emphasizes a contract-first workflow with OpenAPI, strict request/response validation, clear layering between HTTP, application logic, and persistence, and safe interaction with transactional databases.


## Goals

- Contract-first development using OpenAPI with generated, type-safe handlers and models.
- Minimal but robust HTTP server wiring with explicit timeouts and middleware.
- Clear separation of concerns: transport, domain/application, and data/persistence.
- Transaction-safe data access with read/write splitting for OLTP workloads.
- Predictable error handling using RFC7807 problem details and sentinel errors.
- Built-in pagination patterns (offset and cursor with signed tokens).
- Simple local development via Docker Compose and `go generate` for codegen.


## Code Generation From OpenAPI Spec

- Specs and codegen configs live under `oapi/`:
  - `oapi/profile-api-spec.yaml`, `oapi/cfg.server.profile.yaml`
  - `oapi/payment-api-spec.yaml`, `oapi/cfg.server.payment.yaml`
- Generated server stubs and models are written to `api/`:
  - `api/profileapi/server.gen.go`
  - `api/paymentapi/server.gen.go`
- The project uses `oapi-codegen` with “std-http-server” and (for Profile) “strict-server” generation.
  - Strict handlers expose request/response objects that enforce spec shapes at compile time.
  - See `server/stdlib_server.go` for how strict handlers are wrapped and registered.
- Regeneration
  - Inline `go:generate` lines are declared in `main.go`:
    - `go tool oapi-codegen -config oapi/cfg.server.profile.yaml oapi/profile-api-spec.yaml`
    - `go tool oapi-codegen -config oapi/cfg.server.payment.yaml oapi/payment-api-spec.yaml`
  - Run `make gen` to regenerate all code (calls `go generate ./...`).
  - Run `make check` to verify generated code is up-to-date.
- Do not edit files under `api/` directly. Change the spec or config in `oapi/` and regenerate.

Validation and error mapping
- Requests are validated against the OpenAPI spec before handler logic via `ProfileHTTPValidationMiddleware` (see `profile-service/middlewares.go`).
- Errors are normalized to RFC7807 problem details (`profile-service/error_handler.go`).


## Source Code Architecture

High-level layout
- `main.go`: Composition root; config parsing, dependency wiring, and server bootstrap.
- `server/`: HTTP server setup using the Go stdlib mux, timeouts, and middleware wiring.
- `oapi/`: OpenAPI specs and `oapi-codegen` configs.
- `api/`: Generated models and server interfaces from OpenAPI.
- `profile-service/`: Domain/application logic, HTTP adapter for the Profile API, middlewares, and migrations.
- `db/`: Database interfaces and abstractions (connection pool, transactions, health, migrations).
- `db/postgres/`: PostgreSQL implementation of `db.ConnectionPool`.
- `compose.yaml`, `.env`: Local development and configuration.

Request flow
1. `server.New` assembles the HTTP mux, attaches middleware, and registers generated handlers.
2. `profile-service/middlewares.go` validates requests against the OpenAPI spec and provides panic recovery.
3. Generated code in `api/profileapi/server.gen.go` or `api/paymentapi/server.gen.go` de/serializes requests and calls your implementation.
4. Profile API implementation lives in `profile-service/api.go` and delegates to application logic in `profile-service/app.go`.
5. Application logic depends on `ProfilePersistence` (an interface) and a `db.ConnectionPool` for transactions and read/write routing.

Key adapters and boundaries
- HTTP → Domain: `profile_service.ProfileAPI` implements `profile_api.StrictServerInterface`.
- Domain → Persistence: `ProfilePersistence` is implemented by `PostgresProfilePersistence` (`profile-service/persistence.go`).
- Persistence → DB: Only uses `db.Querier` so it can work with both `*sqlx.DB` and `*sqlx.Tx`.


## Transactional Database Implementations

Interfaces (`db/db.go`)
- `ConnectionPool` combines health, connection management, migrations, and transaction helpers.
- `ConnectionManager` exposes `Writer()` and `Reader()` for write/read paths.
- `TxManager` exposes `WithTx(ctx, fn)` and `WithTimeoutTx(ctx, timeout, fn)` to run work atomically.
- `Querier` is `sqlx.ExtContext` so both `*sqlx.DB` and `*sqlx.Tx` conform.

PostgreSQL adapter (`db/postgres/postgres.go`)
- Provides a writer `*sqlx.DB` and an optional set of readers with simple random selection for reads.
- Transaction helpers wrap `BEGIN`/`COMMIT`/`ROLLBACK` with panic safety and proper error propagation.
- `HealthCheck()` pings the database via a lightweight query.
- Migrations are intentionally stubbed for teams to plug in their tool of choice (dbmate, goose, atlas).

Application usage (`profile-service/app.go`)
- Writes run inside transactions using `pool.WithTimeoutTx`:
  - Example: `CreateProfile` starts a transaction, calls persistence, and commits or rolls back on error.
- Reads are routed to replicas by calling `pool.Reader()` from persistence methods like `GetProfilesByOffset`/`GetProfilesFirstPage`.

Persistence layer (`profile-service/persistence.go`)
- Uses plain SQL with `sqlx` and returns domain models.
- Maps Postgres constraint violations (e.g., `pgerrcode.UniqueViolation` 23505) to sentinel errors (`ErrDuplicateEntry`).
- Defers policy decisions to the application layer where domain errors are chosen and then mapped to RFC7807.


## Patterns

- Contract-first API
  - OpenAPI spec drives the HTTP surface area and type-safe handlers.
  - Strict handlers prevent shape drift between spec and implementation.
- RFC7807 error model
  - Centralized mapping from domain and validation errors to problem+json (`profile-service/error_handler.go`).
- Middleware-first validation and recovery
  - `ProfileHTTPValidationMiddleware` validates path/query/body and returns 422 for body schema violations.
  - `RecoverHTTPMiddleware` returns a sanitized 500 on panics.
- Dependency injection via constructors and options
  - `server.New(host, port, ...ServerOptions)` and `WithX` option functions.
  - `NewProfileService(pool, persistence, signer)` composes domain dependencies explicitly.
- Read/write splitting and transactions
  - `ConnectionPool.Reader()` for scale-out reads; `Writer()` for writes and cross-entity reads-in-tx.
  - `WithTx` and `WithTimeoutTx` enforce atomic writes with deadlines.
- Pagination
  - Offset pagination with total count for simple list endpoints.
  - Cursor pagination using a stable tuple `(created_at DESC, id DESC)` and HMAC-signed opaque tokens.
- Mapping layer
  - Explicit mapping between domain models and API DTOs in `profile-service/api.go`.
  - Nullable handling enabled in codegen `output-options.nullable-type: true` and helpers in `api/serde`.
- Configuration and security
  - Postgres set via `POSTGRES_PRIMARY_*` and optional `POSTGRES_REPLICA_*` variables.
  - Cursor tokens signed using an HMAC secret (`HMAC_SECRET`), kept out of VCS.


## Local Development

Prerequisites
- Go 1.25+
- Docker + Docker Compose (for Postgres)

Setup
- Start database: `docker compose up -d database`
- Ensure environment variables (see `.env`):
  - `POSTGRES_PRIMARY_HOST`, `POSTGRES_PRIMARY_PORT`, `POSTGRES_PRIMARY_USER`, `POSTGRES_PRIMARY_PASSWORD`, `POSTGRES_PRIMARY_DATABASE`
  - Optional replicas: `POSTGRES_REPLICA_*`
  - `HMAC_SECRET`

Generate and run
- Generate code: `make gen`
- Verify generation: `make check`
- Run locally: `go run .`
- Build binaries: `go build ./...`

Lint and test
- Basic lint: `gofmt -s -w .` and `go vet ./...`
- Run tests: `go test ./...`


## Extending The Template

Add a new API surface
- Author or update the OpenAPI spec under `oapi/your-api-spec.yaml`.
- Add a matching codegen config `oapi/cfg.server.yourapi.yaml` with output to `api/yourapi/server.gen.go`.
- Implement the generated server interface in a new package (e.g., `your-service/`) following the profile service layout.
- Register the implementation in the HTTP server via a `server.WithYourApi(...)` option.
- Run `make gen` and `go run .`.

Add persistence for a new aggregate
- Define a `YourPersistence` interface in your service package using `db.Querier`.
- Implement it in `your-service/persistence.go` with SQL and sentinel error mapping.
- Use `pool.WithTx`/`WithTimeoutTx` for writes; `pool.Reader()` for read paths.
