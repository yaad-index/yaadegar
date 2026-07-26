// Package server is the yaadegar HTTP API server. Per ADR-0001 yaadegar is
// API-first; this skeleton wires the server lifecycle (graceful start + shutdown)
// and a single operational liveness route. Feature routes, the storage
// repository interface, and request handlers land in later issues.
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

// New builds a server bound to addr. The mux carries only /healthz for now
// (operational liveness — not a feature route); feature routes are wired in
// later issues.
func New(addr string, logger *slog.Logger) *Server {
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           routes(),
			ReadHeaderTimeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// routes builds the HTTP handler. v1 exposes only the liveness endpoint.
func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
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
