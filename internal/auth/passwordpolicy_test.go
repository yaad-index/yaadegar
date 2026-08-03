package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
)

// TestValidatePasswordPolicy checks the shared policy floor (ADR-0011 §4).
func TestValidatePasswordPolicy(t *testing.T) {
	// Too short — one below the minimum — is rejected with the named error.
	err := auth.ValidatePasswordPolicy(strings.Repeat("a", auth.MinPasswordLen-1))
	assert.ErrorIs(t, err, auth.ErrPasswordTooShort)

	// Exactly the minimum, and longer, pass.
	assert.NoError(t, auth.ValidatePasswordPolicy(strings.Repeat("a", auth.MinPasswordLen)))
	assert.NoError(t, auth.ValidatePasswordPolicy(strings.Repeat("a", auth.MinPasswordLen+20)))
	assert.ErrorIs(t, auth.ValidatePasswordPolicy(""), auth.ErrPasswordTooShort)
}

// TestHashNewPassword checks the single funnel: it enforces the policy before
// hashing, and its output verifies against the plaintext.
func TestHashNewPassword(t *testing.T) {
	// A policy-violating password never gets hashed.
	_, err := auth.HashNewPassword("short")
	assert.ErrorIs(t, err, auth.ErrPasswordTooShort)

	// A valid password hashes, and the hash verifies.
	hash, err := auth.HashNewPassword("a-long-enough-password")
	require.NoError(t, err)
	ok, err := auth.VerifyPassword("a-long-enough-password", hash)
	require.NoError(t, err)
	assert.True(t, ok)
}
