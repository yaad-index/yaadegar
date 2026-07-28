package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yaad-index/yaadegar/internal/clock"
)

// Role is an authenticated principal's role (ADR-0005 §6). The public giver
// surface is anonymous and carries no role.
type Role string

const (
	// RoleOwner is a tenant-bound list owner — the only role issued in Cut A1.
	RoleOwner Role = "owner"
	// RoleSuperadmin is the instance-level administrator. The value exists in A1 so
	// the claims model is stable, but no superadmin is bootstrapped and the
	// admin-surface carve-out from the tenant-match invariant lands with A2 — so in
	// A1 every issued token is an owner token (ADR-0005 §5/§6).
	RoleSuperadmin Role = "superadmin"
)

// SuperadminTenant is the sentinel tenant id a superadmin token carries. The
// instance-level superadmin is not tenant-scoped (ADR-0005 §6), but Claims require
// a non-empty tid, so this deliberately-synthetic value stands in. It is NOT a UUID
// — and tenant ids always are (storage mints them with uuid) — so it can never
// equal a real tenant id. That guarantee is load-bearing: the owner surface's
// tenant-match check therefore always rejects a superadmin token, independently of
// the role check. If tenant ids ever stop being UUIDs, revisit this value.
const SuperadminTenant = "__superadmin__"

// signingMethod is the one algorithm this service signs and accepts. Pinning it
// (rather than trusting a token's own `alg` header) is the defense against the
// alg-confusion / `alg:none` attack — ADR-0005 §5.
var signingMethod = jwt.SigningMethodHS256

// Principal is the validated identity a token resolves to.
type Principal struct {
	UserID   string
	TenantID string
	Role     Role
}

// Claims is the JWT payload: the standard registered claims plus the tenant id and
// role. `sub` carries the user id.
type Claims struct {
	TenantID string `json:"tid"`
	Role     Role   `json:"role"`
	jwt.RegisteredClaims
}

// Issuer mints and validates session JWTs with a fixed HMAC secret and access-token
// lifetime. The clock is injected so expiry is testable without real time.
type Issuer struct {
	secret    []byte
	issuerID  string
	accessTTL time.Duration
	clock     clock.Clock
}

// NewIssuer builds an Issuer. secret must already be validated (see Config); ttl is
// the access-token lifetime; clk defaults to the real clock.
func NewIssuer(secret []byte, issuerID string, ttl time.Duration, clk clock.Clock) *Issuer {
	if clk == nil {
		clk = clock.Real{}
	}
	return &Issuer{secret: secret, issuerID: issuerID, accessTTL: ttl, clock: clk}
}

// AccessTTL is the access-token lifetime this issuer stamps.
func (i *Issuer) AccessTTL() time.Duration { return i.accessTTL }

// Issue mints a signed access token for the principal.
func (i *Issuer) Issue(p Principal) (string, error) {
	now := i.clock.Now()
	claims := Claims{
		TenantID: p.TenantID,
		Role:     p.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   p.UserID,
			Issuer:    i.issuerID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
		},
	}
	tok := jwt.NewWithClaims(signingMethod, claims)
	signed, err := tok.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

// ErrInvalidToken is returned for any token that fails validation (bad signature,
// wrong/absent algorithm, expired, or malformed). Callers map it to 401 without
// leaking which check failed.
var ErrInvalidToken = errors.New("auth: invalid token")

// Validate verifies signature, algorithm, and time claims, returning the resolved
// principal. The parser is pinned to HS256 via WithValidMethods, so `alg:none` and
// any non-HS256 algorithm are rejected before the key function runs; the key
// function additionally asserts the concrete method as defense in depth.
func (i *Issuer) Validate(tokenString string) (Principal, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method", ErrInvalidToken)
			}
			return i.secret, nil
		},
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithIssuer(i.issuerID),
		jwt.WithTimeFunc(i.clock.Now),
	)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.Subject == "" || claims.TenantID == "" || claims.Role == "" {
		return Principal{}, fmt.Errorf("%w: missing required claims", ErrInvalidToken)
	}
	return Principal{UserID: claims.Subject, TenantID: claims.TenantID, Role: claims.Role}, nil
}
