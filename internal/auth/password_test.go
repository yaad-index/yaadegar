package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	assert.Contains(t, hash, "$argon2id$")

	ok, err := auth.VerifyPassword("correct horse battery staple", hash)
	require.NoError(t, err)
	assert.True(t, ok, "correct password should verify")

	ok, err = auth.VerifyPassword("wrong password", hash)
	require.NoError(t, err)
	assert.False(t, ok, "wrong password must not verify")
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	a, err := auth.HashPassword("same")
	require.NoError(t, err)
	b, err := auth.HashPassword("same")
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "each hash must use a fresh random salt")
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	_, err := auth.VerifyPassword("x", "not-a-phc-string")
	require.ErrorIs(t, err, auth.ErrInvalidHash)
}
