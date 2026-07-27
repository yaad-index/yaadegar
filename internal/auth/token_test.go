package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
)

const testSecret = "test-signing-secret-of-sufficient-length-000000"

var epoch = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

func testIssuer(clk clock.Clock) *auth.Issuer {
	return auth.NewIssuer([]byte(testSecret), "yaadegar", time.Hour, clk)
}

func TestIssueValidateRoundtrip(t *testing.T) {
	i := testIssuer(clock.NewFake(epoch))
	tok, err := i.Issue(auth.Principal{UserID: "u1", TenantID: "t1", Role: auth.RoleOwner})
	require.NoError(t, err)

	p, err := i.Validate(tok)
	require.NoError(t, err)
	assert.Equal(t, "u1", p.UserID)
	assert.Equal(t, "t1", p.TenantID)
	assert.Equal(t, auth.RoleOwner, p.Role)
}

func TestValidateRejectsExpired(t *testing.T) {
	clk := clock.NewFake(epoch)
	i := testIssuer(clk)
	tok, err := i.Issue(auth.Principal{UserID: "u1", TenantID: "t1", Role: auth.RoleOwner})
	require.NoError(t, err)

	clk.Advance(2 * time.Hour) // past the 1h TTL
	_, err = i.Validate(tok)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateRejectsWrongSecret(t *testing.T) {
	signer := testIssuer(clock.NewFake(epoch))
	tok, err := signer.Issue(auth.Principal{UserID: "u1", TenantID: "t1", Role: auth.RoleOwner})
	require.NoError(t, err)

	other := auth.NewIssuer([]byte("a-different-secret-of-sufficient-length-00000"), "yaadegar", time.Hour, clock.NewFake(epoch))
	_, err = other.Validate(tok)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

// TestValidateRejectsNoneAlg is the load-bearing algorithm-pinning test
// (ADR-0005 §5): a token with `alg: none` — the classic none-algorithm attack —
// must be rejected, never trusting the token's own alg header.
func TestValidateRejectsNoneAlg(t *testing.T) {
	claims := auth.Claims{
		TenantID: "t1",
		Role:     auth.RoleOwner,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u1",
			Issuer:    "yaadegar",
			ExpiresAt: jwt.NewNumericDate(epoch.Add(time.Hour)),
		},
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tok, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	i := testIssuer(clock.NewFake(epoch))
	_, err = i.Validate(tok)
	require.ErrorIs(t, err, auth.ErrInvalidToken, "alg:none must be rejected")
}

func TestValidateRejectsWrongIssuer(t *testing.T) {
	foreign := auth.NewIssuer([]byte(testSecret), "somebody-else", time.Hour, clock.NewFake(epoch))
	tok, err := foreign.Issue(auth.Principal{UserID: "u1", TenantID: "t1", Role: auth.RoleOwner})
	require.NoError(t, err)

	i := testIssuer(clock.NewFake(epoch))
	_, err = i.Validate(tok)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}
