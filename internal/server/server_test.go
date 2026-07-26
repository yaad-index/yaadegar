package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server serves /healthz and shuts down cleanly when its context is
// cancelled — the skeleton lifecycle later issues build on.
func TestServer_ServesHealthzThenShutsDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := New("", slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok\n", string(body))

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err, "graceful shutdown returns no error")
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within the timeout")
	}
}

// An unknown path is a 404 — only /healthz is wired in the skeleton.
func TestServer_UnknownRouteIs404(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := New("", slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/nope")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
