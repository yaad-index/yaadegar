// Package server owns the yaadegar HTTP lifecycle: graceful start and shutdown
// around an injected handler. The handler itself (routing, tenancy, auth, the
// API surface) is built by internal/api and passed in, keeping this package
// concerned only with process lifecycle.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// shutdownTimeout bounds how long a graceful shutdown waits for in-flight
// requests to drain before the server is forced closed.
const shutdownTimeout = 10 * time.Second

// Server owns the HTTP lifecycle.
type Server struct {
	http   *http.Server
	logger *slog.Logger
}

// New builds a server bound to addr that serves the given handler.
func New(addr string, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Run listens on the configured address and serves until ctx is cancelled, then
// shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve serves on an already-open listener until ctx is cancelled, then shuts
// down gracefully. Splitting the listener out lets tests bind an ephemeral port.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "addr", ln.Addr().String())
		errCh <- s.http.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down http server")
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return s.http.Shutdown(shutCtx)
	}
}
