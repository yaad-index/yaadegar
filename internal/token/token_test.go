package token_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/token"
)

func TestAPersonalAccessTokenCarriesItsPrefix(t *testing.T) {
	raw, hash, err := token.NewPAT()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, token.PATPrefix))
	assert.Greater(t, len(raw), len(token.PATPrefix)+40, "the prefix is followed by 32 random bytes")
	assert.Equal(t, token.Hash(raw), hash, "the hash covers the whole raw value, prefix included")
	assert.NotContains(t, hash, raw)
	assert.True(t, token.IsPAT(raw))

	raw2, _, err := token.NewPAT()
	require.NoError(t, err)
	assert.NotEqual(t, raw, raw2)
}

func TestIsPATOnlyMatchesThePrefix(t *testing.T) {
	plain, _, err := token.New()
	require.NoError(t, err)
	for _, c := range []string{plain, "", "ydg_pa", "eyJhbGciOiJIUzI1NiJ9.e30.x", " ydg_pat_x"} {
		assert.False(t, token.IsPAT(c), c)
	}
}
