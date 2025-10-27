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

package server

import (
	payment_api "app/api/paymentapi"
	profile_api "app/api/profileapi"
	profile_service "app/profile-service"
	"log/slog"

	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

const MAX_TCP_PORT = 1 << 16 // A TCP header uses a 16-bit field for port numbers

type (
	Server struct {
		profileApi profile_api.StrictServerInterface
		paymentApi payment_api.ServerInterface

		server *http.Server
		host   string
		port   uint16
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

func WithPaymentApi(si payment_api.ServerInterface) ServerOptions {
	return func(s *Server) {
		s.paymentApi = si
	}
}

func WithProfileApi(si profile_api.StrictServerInterface) ServerOptions {
	return func(s *Server) {
		s.profileApi = si
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

	for _, opt := range opts {
		opt(s)
	}

	mux := http.NewServeMux()

	// TODO: feature flag this
	//
	// For now, register all the apis
	//
	// Risks path collision if the profile api and payment apis specs
	// are of two different files (different teams, etc.)
	if s.profileApi != nil {
		// Build a "strict" handler for the profile API which wraps our
		// implementation with request/response de/serialization and
		// RFC7807 error mapping.
		//
		// This returns a profile_api.ServerInterface that the stdlib router can use.
		strict := profile_api.NewStrictHandlerWithOptions(
			s.profileApi,
			[]profile_api.StrictMiddlewareFunc{},
			profile_api.StrictHTTPServerOptions{
				RequestErrorHandlerFunc:  profile_service.ProblemDetailsRequestErrorHandler,
				ResponseErrorHandlerFunc: profile_service.ProblemDetailsResponseErrorHandler,
			},
		)

		// Register routes onto our existing mux.
		//
		// Note: HandlerWithOptions returns an http.Handler, but when
		// StdHTTPServerOptions.BaseRouter is provided, it mutates that
		// router in place (attaches HandleFunc bindings) and returns the
		// same router.
		//
		// Because we pass BaseRouter: mux below, the registration
		// side-effects occur on "mux" directly, so it is both
		// safe and intentional to ignore the returned handler here.
		//
		// If BaseRouter were nil, HandlerWithOptions would allocate a new
		// *http.ServeMux and return it, and in that case we would need to
		// capture and use the returned handler.
		profile_api.HandlerWithOptions(
			strict,
			profile_api.StdHTTPServerOptions{
				BaseRouter:       mux,
				Middlewares:      []profile_api.MiddlewareFunc{},
				ErrorHandlerFunc: profile_service.ProblemDetailsRequestErrorHandler,
			},
		)

		slog.Info("configured profile api")

	}
	if s.paymentApi != nil {
		payment_api.HandlerFromMux(s.paymentApi, mux)

		slog.Info("configured payment api")
	}

	// Wrap the entire mux so validation (sets defaults) runs before param binding,
	validated := profile_service.ProfileHTTPValidationMiddleware()(mux)
	// then apply global panic-recover middleware.
	s.server.Handler = profile_service.RecoverHTTPMiddleware()(validated)

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
