// Package oauthlogin implements the protocol mechanics of owner login via an
// OpenID Connect provider (Google in v1, ADR-0008): the Authorization-Code +
// PKCE flow and strict ID-token verification (delegated to go-oidc, never
// hand-rolled), plus the HMAC-signed state and one-time ticket that carry the
// flow across hosts. It holds no storage and no session concept — the API layer
// drives it, resolves the owner, and issues the session.
package oauthlogin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// GoogleIssuerURL is the OIDC issuer for Google accounts. It is the default and
// the value the ID token's `iss` is verified against (go-oidc pins iss to the
// discovered issuer, so this doubles as the ADR-0008 §4 iss check). It is a
// field, not a constant, only so tests can point the flow at a mock provider.
const GoogleIssuerURL = "https://accounts.google.com"

// Config configures the provider client. The secret arrives from the environment
// (never a file); HMACKey signs the state cookie and the handoff ticket.
type Config struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is the single, fixed callback URL registered with the provider
	// (ADR-0008 §2): YAADEGAR_OAUTH_REDIRECT_BASE + the callback path.
	RedirectURL string
	// IssuerURL is the OIDC issuer (defaults to GoogleIssuerURL). Overridable for
	// tests against a mock provider; in production it is Google's.
	IssuerURL string
	// HMACKey signs the state cookie and the one-time ticket. It must be
	// high-entropy; the instance reuses the validated JWT signing secret.
	HMACKey []byte
}

// Identity is the verified subset of an ID token the account model needs.
type Identity struct {
	// Subject is the provider's stable, opaque user id (Google `sub`) — the durable
	// link key, never the email.
	Subject string
	Email   string
	// EmailVerified is the provider's assertion that it controls the mailbox. The
	// account model requires it true before linking (ADR-0008 §5, guard 1).
	EmailVerified bool
}

// Authenticator wraps the provider client: the OAuth2 config for the code
// exchange and the go-oidc verifier for ID-token validation.
type Authenticator struct {
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	signer   Signer
}

// New builds an Authenticator, performing OIDC discovery against the issuer (a
// network call). A discovery failure is returned so startup can fail closed.
func New(ctx context.Context, cfg Config) (*Authenticator, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("oauthlogin: ClientID, ClientSecret, and RedirectURL are required")
	}
	if len(cfg.HMACKey) == 0 {
		return nil, errors.New("oauthlogin: HMACKey is required")
	}
	issuer := cfg.IssuerURL
	if issuer == "" {
		issuer = GoogleIssuerURL
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oauthlogin: OIDC discovery on %s: %w", issuer, err)
	}
	return &Authenticator{
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		signer:   NewSigner(cfg.HMACKey),
	}, nil
}

// Signer exposes the HMAC codec so the caller can sign/verify state and tickets
// with the same key material.
func (a *Authenticator) Signer() Signer { return a.signer }

// AuthCodeURL builds the provider redirect for the Authorization-Code flow with
// PKCE (S256) and the OIDC nonce. state is the opaque CSRF value echoed back;
// pkceVerifier is the high-entropy verifier whose S256 challenge is sent now and
// whose plaintext is replayed at Exchange.
func (a *Authenticator) AuthCodeURL(state, nonce, pkceVerifier string) string {
	return a.oauth.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(pkceVerifier),
	)
}

// Exchange trades the authorization code for tokens, replaying the PKCE verifier,
// and returns the raw ID token. It never trusts the access token for identity.
func (a *Authenticator) Exchange(ctx context.Context, code, pkceVerifier string) (string, error) {
	tok, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(pkceVerifier))
	if err != nil {
		return "", fmt.Errorf("oauthlogin: code exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return "", errors.New("oauthlogin: token response carried no id_token")
	}
	return rawID, nil
}

// VerifyIDToken runs the full ID-token verification (ADR-0008 §4): go-oidc checks
// the JWKS signature, `iss` (against the discovered issuer), `aud` (against our
// client id), and `exp`; this then enforces the `nonce` binding before returning
// the identity. Any failure aborts the login.
func (a *Authenticator) VerifyIDToken(ctx context.Context, rawIDToken, nonce string) (Identity, error) {
	idt, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("oauthlogin: verify id_token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(nonce)) != 1 {
		return Identity{}, errors.New("oauthlogin: id_token nonce mismatch")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("oauthlogin: decode id_token claims: %w", err)
	}
	return Identity{
		Subject:       idt.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
	}, nil
}

// RandomToken returns n bytes of URL-safe base64 randomness (state, nonce, jti).
// It panics only if the OS CSPRNG fails, which is unrecoverable.
func RandomToken(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic("oauthlogin: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// NewPKCEVerifier mints a fresh PKCE code verifier (RFC 7636), using the x/oauth2
// generator so the format matches S256ChallengeOption/VerifierOption exactly.
func NewPKCEVerifier() string { return oauth2.GenerateVerifier() }

// Signer signs and verifies short, self-contained payloads with HMAC-SHA256. It
// backs both the browser state cookie and the cross-host ticket; a `typ` field in
// each payload (see StatePayload/Ticket) domain-separates the two so one can never
// be replayed as the other.
type Signer struct{ key []byte }

// NewSigner builds a Signer over the given key material.
func NewSigner(key []byte) Signer { return Signer{key: key} }

// ErrBadSignature is returned when a token's payload/signature do not verify.
var ErrBadSignature = errors.New("oauthlogin: bad signature")

// sign returns base64url(payload) + "." + base64url(HMAC(payload)).
func (s Signer) sign(payload []byte) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verify checks the signature and returns the raw payload.
func (s Signer) verify(token string) ([]byte, error) {
	payloadB64, sigB64, ok := strings.Cut(token, ".")
	if !ok {
		return nil, ErrBadSignature
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, ErrBadSignature
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, ErrBadSignature
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, ErrBadSignature
	}
	return payload, nil
}

// StatePayload is the signed browser cookie set at /start and read at /callback
// (ADR-0008 §8). It never leaves the fixed host and is not a session.
type StatePayload struct {
	Typ string `json:"typ"` // always "state"
	// State is the opaque CSRF value echoed to the provider and compared on return.
	State string `json:"s"`
	// Nonce binds the ID token to this flow.
	Nonce string `json:"n"`
	// PKCEVerifier is replayed at the code exchange.
	PKCEVerifier string `json:"v"`
	// TenantID is the tenant resolved at /start; the callback trusts this, never a
	// request-supplied host.
	TenantID string `json:"t"`
	// TenantHost is the validated host the login began on — the ticket bounce
	// target (works for subdomains and custom domains alike).
	TenantHost string `json:"h"`
	// ReturnTo is the local post-login path.
	ReturnTo string `json:"r"`
	// Exp is the cookie's own expiry (unix seconds); the caller checks it.
	Exp int64 `json:"e"`
}

const typState = "state"

// SignState marshals and signs a state payload (stamping its type).
func (s Signer) SignState(p StatePayload) string {
	p.Typ = typState
	b, _ := json.Marshal(p)
	return s.sign(b)
}

// VerifyState verifies and unmarshals a state cookie, rejecting a payload that is
// not of the state type (so a ticket can never be replayed here).
func (s Signer) VerifyState(token string) (StatePayload, error) {
	b, err := s.verify(token)
	if err != nil {
		return StatePayload{}, err
	}
	var p StatePayload
	if err := json.Unmarshal(b, &p); err != nil {
		return StatePayload{}, ErrBadSignature
	}
	if p.Typ != typState {
		return StatePayload{}, ErrBadSignature
	}
	return p, nil
}

// Ticket is the one-time, short-TTL cross-host handoff minted by /callback on the
// fixed host and consumed by /complete on the tenant host (ADR-0008 §3). Jti makes
// it single-use against the consumed-jti guard.
type Ticket struct {
	Typ      string `json:"typ"` // always "ticket"
	TenantID string `json:"t"`
	UserID   string `json:"u"`
	ReturnTo string `json:"r"`
	Jti      string `json:"j"`
	Exp      int64  `json:"e"`
}

const typTicket = "ticket"

// SignTicket marshals and signs a ticket (stamping its type).
func (s Signer) SignTicket(t Ticket) string {
	t.Typ = typTicket
	b, _ := json.Marshal(t)
	return s.sign(b)
}

// VerifyTicket verifies and unmarshals a ticket, rejecting a payload that is not
// of the ticket type (so a state cookie can never be replayed here).
func (s Signer) VerifyTicket(token string) (Ticket, error) {
	b, err := s.verify(token)
	if err != nil {
		return Ticket{}, err
	}
	var t Ticket
	if err := json.Unmarshal(b, &t); err != nil {
		return Ticket{}, ErrBadSignature
	}
	if t.Typ != typTicket {
		return Ticket{}, ErrBadSignature
	}
	return t, nil
}
