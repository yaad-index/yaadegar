package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
)

// TestNewServiceFailClosed is the load-bearing startup invariant (ADR-0005 §4):
// a missing/short secret or no enabled method must refuse to construct.
func TestNewServiceFailClosed(t *testing.T) {
	clk := clock.Real{}
	goodSecret := strings.Repeat("x", auth.MinSecretLen)

	t.Run("missing secret", func(t *testing.T) {
		_, err := auth.NewService(auth.Config{PasswordEnabled: true}, clk)
		require.Error(t, err)
	})
	t.Run("short secret", func(t *testing.T) {
		_, err := auth.NewService(auth.Config{JWTSecret: "too-short", PasswordEnabled: true}, clk)
		require.Error(t, err)
	})
	t.Run("no method enabled", func(t *testing.T) {
		_, err := auth.NewService(auth.Config{JWTSecret: goodSecret}, clk)
		require.Error(t, err)
	})
	t.Run("valid: secret + password", func(t *testing.T) {
		svc, err := auth.NewService(auth.Config{JWTSecret: goodSecret, PasswordEnabled: true}, clk)
		require.NoError(t, err)
		assert.True(t, svc.Enabled(auth.MethodPassword))
		require.NotNil(t, svc.Issuer())
	})
}
