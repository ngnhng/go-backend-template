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

package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"
)

const MAX_TCP_PORT = 1 << 16 // A TCP header uses a 16-bit field for port numbers

type (
	Server struct {
		server *http.Server
		mux    *http.ServeMux
		host   string
		port   uint16

		// global middleware chain applied around the mux
		middlewares []func(http.Handler) http.Handler

		// registrable services that mount routes and provide their own middlewares
		services []RegistrableService
	}

	ServerOptions func(*Server)
)

func WithWriteTimeout(t time.Duration) ServerOptions {
	return func(s *Server) {
		if t != 0 {
			s.server.WriteTimeout = t
		} else {
			s.server.WriteTimeout = 10 * time.Second
		}
	}
}

func WithReadTimeout(t time.Duration) ServerOptions {
	return func(s *Server) {
		if t != 0 {
			s.server.ReadTimeout = t
		} else {
			s.server.ReadTimeout = 10 * time.Second
		}
	}
}

// WithServices registers a collection of self-contained, registrable services.
func WithServices(svcs ...RegistrableService) ServerOptions {
	return func(s *Server) {
		if len(svcs) > 0 {
			s.services = append(s.services, svcs...)
		}
	}
}

// WithGlobalMiddlewares registers global middlewares wrapping the entire server mux.
// The middlewares are applied in the order provided.
func WithGlobalMiddlewares(mw ...func(http.Handler) http.Handler) ServerOptions {
	return func(s *Server) {
		if len(mw) == 0 {
			return
		}
		s.middlewares = append(s.middlewares, mw...)
	}
}

// Example usage:
//
//	server, _ := New("0.0.0.0", 8080, WithWriteTimeout(10*time.Second))
func New(host string, port int, opts ...ServerOptions) (*Server, error) {
	if len(host) == 0 {
		// the server binds to 0.0.0.0, which instructs the OS to listen for
		// inbound connections from all network interfaces on the host
		// machine (loopback, Wi-Fi adapters, etc.)
		slog.Warn("empty host, binding to all interfaces")
		host = "0.0.0.0"
	}
	if port <= 0 || port > MAX_TCP_PORT {
		return nil, fmt.Errorf("bad port")
	}
	s := &Server{
		host: host,
		port: uint16(port),
	}

	s.server = &http.Server{
		Addr: net.JoinHostPort(host, strconv.Itoa(port)),
	}
	// Allocate a base mux before applying options so options can register routes.
	s.mux = http.NewServeMux()

	for _, opt := range opts {
		opt(s)
	}

	// Register all services and collect their required global middlewares.
	for _, svc := range s.services {
		svc.Register(s.mux)
		s.middlewares = append(s.middlewares, svc.Middlewares()...)
		slog.Info("registered service", slog.String("type", fmt.Sprintf("%T", svc)))
	}

	// Build handler chain: middlewares wrap the mux in declaration order.
	handler := http.Handler(s.mux)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}
	// Attach the composed handler chain. Consumers can add recover/logging via options.
	s.server.Handler = handler

	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	done := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "started server", slog.Any("host", s.host), slog.Any("port", s.port))
		if err := s.server.ListenAndServe(); err != nil {
			errCh <- err
			return
		}
	}()

	go func() {
		for {
			select {
			case e := <-errCh:
				slog.ErrorContext(ctx, "server error", slog.Any("error", e))
				done <- struct{}{}
			case <-ctx.Done():
				done <- struct{}{}
			}
		}
	}()

	<-done
	slog.InfoContext(ctx, "shutting down...")
	dCtx, dCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dCancel()
	// allows 10 seconds for graceful shutdown
	return s.server.Shutdown(dCtx)
}
