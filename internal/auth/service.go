package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/yaad-index/yaadegar/internal/clock"
)

// Method identifies a login method (ADR-0005 §3). Only password ships in Cut A1;
// magic-link (Cut B) and Google OAuth (Cut C) extend this.
type Method string

const (
	MethodPassword Method = "password"
)

// MinSecretLen is the minimum accepted JWT signing-secret length in bytes (256-bit
// floor for HS256). Shorter or absent fails closed at startup.
const MinSecretLen = 32

// DefaultAccessTTL is the access-token lifetime when unset — a moderate window,
// re-login on expiry (access-only until refresh lands in Cut A′, ADR-0005 §5).
const DefaultAccessTTL = 12 * time.Hour

// DefaultIssuer is the `iss` claim value when unset.
const DefaultIssuer = "yaadegar"

// Config is the operator-facing auth configuration. Secrets arrive from the
// environment (never a config file); the API/CLI layer populates this.
type Config struct {
	// JWTSecret signs and verifies session tokens (HS256). Required and at least
	// MinSecretLen bytes, else the instance refuses to start.
	JWTSecret string
	// Issuer is the `iss` claim (defaults to DefaultIssuer).
	Issuer string
	// AccessTTL is the access-token lifetime (defaults to DefaultAccessTTL).
	AccessTTL time.Duration

	// PasswordEnabled turns on username+password login. Cut B/C add the other
	// methods' flags + their required settings here.
	PasswordEnabled bool
}

// Service is the validated auth core: a token issuer plus the set of enabled login
// methods. It is constructed once at startup by NewService, which enforces the
// fail-closed invariants; if it returns an error the instance must not serve.
type Service struct {
	issuer  *Issuer
	enabled map[Method]bool
}

// NewService validates cfg and builds the Service, enforcing ADR-0005 §4's
// fail-closed invariants:
//   - the JWT signing secret is present and at least MinSecretLen bytes;
//   - at least one login method is enabled AND its required configuration is valid.
//
// Any violation is a returned error naming the problem so the operator can fix it;
// the caller (startup) aborts rather than serving in an un-authenticatable state.
func NewService(cfg Config, clk clock.Clock) (*Service, error) {
	if len(cfg.JWTSecret) < MinSecretLen {
		return nil, fmt.Errorf(
			"auth: a JWT signing secret of at least %d bytes is required (set YAADEGAR_AUTH_JWT_SECRET); refusing to start",
			MinSecretLen)
	}

	enabled := map[Method]bool{}
	if cfg.PasswordEnabled {
		// Password has no extra required configuration; enabling it is sufficient.
		enabled[MethodPassword] = true
	}
	if len(enabled) == 0 {
		return nil, errors.New(
			"auth: no authentication method is enabled — enable at least one (e.g. password); refusing to start")
	}

	ttl := cfg.AccessTTL
	if ttl <= 0 {
		ttl = DefaultAccessTTL
	}
	issuerID := cfg.Issuer
	if issuerID == "" {
		issuerID = DefaultIssuer
	}
	return &Service{
		issuer:  NewIssuer([]byte(cfg.JWTSecret), issuerID, ttl, clk),
		enabled: enabled,
	}, nil
}

// Issuer returns the token issuer/validator.
func (s *Service) Issuer() *Issuer { return s.issuer }

// Enabled reports whether a login method is enabled.
func (s *Service) Enabled(m Method) bool { return s.enabled[m] }
