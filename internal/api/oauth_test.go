package api_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/oauthlogin"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// --- mock OIDC provider -----------------------------------------------------

// mockIdentity is the identity a mock login will assert.
type mockIdentity struct {
	sub           string
	email         string
	emailVerified bool
}

// mockOIDC is a minimal, hermetic OpenID Connect provider: discovery + JWKS +
// authorize + token, signing RS256 id_tokens with an in-test RSA key. It lets the
// OAuth flow be exercised end-to-end without a live Google (ADR-0008 Cut 1).
type mockOIDC struct {
	server   *httptest.Server
	key      *rsa.PrivateKey
	kid      string
	clientID string

	// next is the identity the next authorize call will bind to its code.
	next mockIdentity
	// codes maps an issued auth code to the nonce + identity captured at authorize.
	codes map[string]codeRecord
}

type codeRecord struct {
	nonce    string
	identity mockIdentity
}

func newMockOIDC(t *testing.T, clientID string) *mockOIDC {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	m := &mockOIDC{key: key, kid: "test-key-1", clientID: clientID, codes: map[string]codeRecord{}}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", m.handleDiscovery)
	mux.HandleFunc("/jwks", m.handleJWKS)
	mux.HandleFunc("/authorize", m.handleAuthorize)
	mux.HandleFunc("/token", m.handleToken)
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockOIDC) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                m.server.URL,
		"authorization_endpoint":                m.server.URL + "/authorize",
		"token_endpoint":                        m.server.URL + "/token",
		"jwks_uri":                              m.server.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (m *mockOIDC) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := m.key.Public().(*rsa.PublicKey)
	writeJSON(w, map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": m.kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	})
}

// handleAuthorize captures the nonce and binds a fresh code to the pending
// identity, then redirects to the caller's redirect_uri (the OAuth dance the
// browser would drive at Google).
func (m *mockOIDC) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := "code-" + oauthlogin.RandomToken(8)
	m.codes[code] = codeRecord{nonce: q.Get("nonce"), identity: m.next}
	redirect := q.Get("redirect_uri") + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(q.Get("state"))
	http.Redirect(w, r, redirect, http.StatusFound)
}

// handleToken exchanges a code for a signed id_token carrying the bound nonce and
// identity. It verifies the PKCE code_verifier against the challenge implicitly by
// trusting the client (the app supplies the verifier; a mismatch would surface as
// a Google-side error in production — not re-implemented here).
func (m *mockOIDC) handleToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	rec, ok := m.codes[r.Form.Get("code")]
	if !ok {
		http.Error(w, "unknown code", http.StatusBadRequest)
		return
	}
	claims := jwt.MapClaims{
		"iss":            m.server.URL,
		"aud":            m.clientID,
		"sub":            rec.identity.sub,
		"email":          rec.identity.email,
		"email_verified": rec.identity.emailVerified,
		"nonce":          rec.nonce,
		"iat":            jwt.NewNumericDate(time.Now()),
		"exp":            jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = m.kid
	signed, err := tok.SignedString(m.key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"access_token": "mock-access",
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     signed,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- OAuth test harness -----------------------------------------------------

type oauthHarness struct {
	t        *testing.T
	h        http.Handler
	store    storage.Store
	tenant   storage.Tenant
	owner    storage.User
	mock     *mockOIDC
	fixedURL string // fixed redirect host base (informational)
}

const oauthClientID = "test-client-id.apps.googleusercontent.com"

// newOAuthHarness builds a handler wired with a mock-OIDC authenticator, one
// tenant with a known owner email, and Google login enabled for that tenant.
func newOAuthHarness(t *testing.T, ownerEmail string, googleEnabled bool) *oauthHarness {
	t.Helper()
	ctx := context.Background()
	mock := newMockOIDC(t, oauthClientID)

	dsn := "file:" + filepath.Join(t.TempDir(), "oauth.db")
	store, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(ctx))
	t.Cleanup(func() { _ = store.Close() })

	tenant, err := store.CreateTenant(ctx, storage.Tenant{Subdomain: "alice"})
	require.NoError(t, err)
	owner, err := store.ForTenant(tenant).Users().Create(ctx, storage.User{Name: "Alice", Email: ownerEmail})
	require.NoError(t, err)
	if googleEnabled {
		require.NoError(t, store.SetTenantOAuthGoogle(ctx, tenant.ID, true))
	}

	clk := clock.NewFake(testClockStart)
	authSvc, err := auth.NewService(auth.Config{JWTSecret: testJWTSecret, PasswordEnabled: true}, clk)
	require.NoError(t, err)

	authn, err := oauthlogin.New(ctx, oauthlogin.Config{
		ClientID:     oauthClientID,
		ClientSecret: "test-secret",
		RedirectURL:  "https://fixed.example.test/api/v1/auth/oauth/google/callback",
		IssuerURL:    mock.server.URL,
		HMACKey:      []byte(testJWTSecret),
	})
	require.NoError(t, err)

	h := api.NewHandler(store, api.Options{
		BaseDomain: baseDomain,
		Logger:     slog.New(slog.DiscardHandler),
		Clock:      clk,
		Auth:       authSvc,
		OAuth:      authn,
	})
	return &oauthHarness{t: t, h: h, store: store, tenant: tenant, owner: owner, mock: mock, fixedURL: "https://fixed.example.test"}
}

// get issues a GET through the handler, carrying the given cookies, and returns
// the recorder (status, headers, Set-Cookie, body).
func (o *oauthHarness) get(path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	return o.getOn("fixed.example.test", path, cookies)
}

// getOn issues a GET with an explicit Host (tenant routing), carrying cookies.
func (o *oauthHarness) getOn(host, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	o.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
	req.Host = host
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, req)
	return rec
}

// tenantHost is the host the login begins on (subdomain under the base domain).
func (o *oauthHarness) tenantHost() string { return o.tenant.Subdomain + "." + baseDomain }

// runToCallback drives /start → mock /authorize and returns the app callback URL
// (code+state) plus the state cookie to present to /callback.
func (o *oauthHarness) runToCallback(t *testing.T, returnTo string) (callbackPath string, stateCookie *http.Cookie) {
	t.Helper()
	startPath := "/api/v1/auth/oauth/google/start?tenant=" + url.QueryEscape(o.tenantHost())
	if returnTo != "" {
		startPath += "&return_to=" + url.QueryEscape(returnTo)
	}
	startRec := o.get(startPath, nil)
	require.Equal(t, http.StatusFound, startRec.Code, "start should redirect to the provider")
	stateCookie = findCookie(startRec.Result().Cookies(), "yaadegar_oauth_state")
	require.NotNil(t, stateCookie, "start must set the state cookie")

	// Follow the redirect to the mock authorize endpoint (the browser→Google hop).
	authorizeURL := startRec.Header().Get("Location")
	require.Contains(t, authorizeURL, o.mock.server.URL+"/authorize")
	authorizeRec := driveGetNoRedirect(t, authorizeURL)
	loc := authorizeRec.Header.Get("Location")
	require.NotEmpty(t, loc, "authorize should redirect back with a code")
	u, err := url.Parse(loc)
	require.NoError(t, err)
	return "/api/v1/auth/oauth/google/callback?" + u.RawQuery, stateCookie
}

// findCookie returns the named cookie from a Set-Cookie slice, or nil.
func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// --- tests ------------------------------------------------------------------

func TestOAuth_HappyPath_FirstLinkThenReturningLogin(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-1", email: "alice@example.com", emailVerified: true}

	// First login: links the verified email to the existing owner and issues a
	// session on the tenant host.
	session := o.completeLogin(t, "/dashboard")
	assert.NotEmpty(t, session.Value, "a session JWT cookie must be set")
	// The session validates as the owner on this tenant.
	principal, err := o.authIssuerValidate(session.Value)
	require.NoError(t, err)
	assert.Equal(t, o.owner.ID, principal.UserID)
	assert.Equal(t, o.tenant.ID, principal.TenantID)
	assert.Equal(t, auth.RoleOwner, principal.Role)

	// The link is now recorded exactly once.
	oi, err := o.store.ForTenant(o.tenant).OAuthIdentities().
		ByProviderSubject(context.Background(), storage.OAuthProviderGoogle, "google-sub-1")
	require.NoError(t, err)
	assert.Equal(t, o.owner.ID, oi.UserID)

	// Second login with the same subject is the returning-login fast path — still
	// the same owner, no duplicate link.
	session2 := o.completeLogin(t, "")
	p2, err := o.authIssuerValidate(session2.Value)
	require.NoError(t, err)
	assert.Equal(t, o.owner.ID, p2.UserID)
}

func TestOAuth_RejectsUnverifiedEmail(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-1", email: "alice@example.com", emailVerified: false}

	callbackPath, stateCookie := o.runToCallback(t, "")
	rec := o.get(callbackPath, []*http.Cookie{stateCookie})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, findCookie(rec.Result().Cookies(), "yaadegar_session"))
}

func TestOAuth_RejectsNoMatchingOwner(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-2", email: "stranger@example.com", emailVerified: true}

	callbackPath, stateCookie := o.runToCallback(t, "")
	rec := o.get(callbackPath, []*http.Cookie{stateCookie})
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestOAuth_RejectsOwnerAlreadyLinkedToDifferentSubject(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	// First: link subject A to the owner.
	o.mock.next = mockIdentity{sub: "google-sub-A", email: "alice@example.com", emailVerified: true}
	o.completeLogin(t, "")

	// Then a DIFFERENT Google account with the same email tries to link — rejected.
	o.mock.next = mockIdentity{sub: "google-sub-B", email: "alice@example.com", emailVerified: true}
	callbackPath, stateCookie := o.runToCallback(t, "")
	rec := o.get(callbackPath, []*http.Cookie{stateCookie})
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestOAuth_CallbackRejectsStateMismatch(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-1", email: "alice@example.com", emailVerified: true}
	callbackPath, stateCookie := o.runToCallback(t, "")
	// Tamper with the state query param so it no longer matches the cookie.
	tampered := strings.Replace(callbackPath, "state=", "state=zzz", 1)
	rec := o.get(tampered, []*http.Cookie{stateCookie})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOAuth_CallbackRequiresStateCookie(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-1", email: "alice@example.com", emailVerified: true}
	callbackPath, _ := o.runToCallback(t, "")
	rec := o.get(callbackPath, nil) // no state cookie
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOAuth_TicketIsSingleUse(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-1", email: "alice@example.com", emailVerified: true}
	callbackPath, stateCookie := o.runToCallback(t, "")
	cbRec := o.get(callbackPath, []*http.Cookie{stateCookie})
	require.Equal(t, http.StatusFound, cbRec.Code)
	completePath := completePathFromLocation(t, cbRec.Header().Get("Location"))

	first := o.get(completePath, nil)
	require.Equal(t, http.StatusFound, first.Code)
	require.NotNil(t, findCookie(first.Result().Cookies(), "yaadegar_session"))

	// Replaying the same ticket is rejected (consumed-jti guard).
	second := o.get(completePath, nil)
	assert.Equal(t, http.StatusBadRequest, second.Code)
	assert.Nil(t, findCookie(second.Result().Cookies(), "yaadegar_session"))
}

func TestOAuth_StartRejectsWhenTenantToggleOff(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", false) // toggle off
	rec := o.get("/api/v1/auth/oauth/google/start?tenant="+url.QueryEscape(o.tenantHost()), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestOAuth_EndpointsAbsentWhenNotConfigured(t *testing.T) {
	// A handler with no OAuth authenticator: every endpoint is 404.
	h := newHarness(t)
	for _, p := range []string{
		"/api/v1/auth/oauth/google/start?tenant=alice.example.test",
		"/api/v1/auth/oauth/google/callback?code=x&state=y",
		"/api/v1/auth/oauth/complete?ticket=x",
	} {
		resp, _ := h.reqH(http.MethodGet, strings.SplitN(p, "?", 2)[0]+queryOf(p), "alice."+baseDomain, nil, nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, p)
	}
}

func TestOAuth_CompleteRejectsInvalidTicket(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	rec := o.get("/api/v1/auth/oauth/complete?ticket=not-a-valid-ticket", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- helpers ----------------------------------------------------------------

// completeLogin runs the full flow (start → authorize → callback → complete) and
// returns the session cookie set by /complete.
func (o *oauthHarness) completeLogin(t *testing.T, returnTo string) *http.Cookie {
	t.Helper()
	callbackPath, stateCookie := o.runToCallback(t, returnTo)
	cbRec := o.get(callbackPath, []*http.Cookie{stateCookie})
	require.Equal(t, http.StatusFound, cbRec.Code, "callback should bounce to the tenant host")
	completePath := completePathFromLocation(t, cbRec.Header().Get("Location"))
	compRec := o.get(completePath, nil)
	require.Equal(t, http.StatusFound, compRec.Code)
	session := findCookie(compRec.Result().Cookies(), "yaadegar_session")
	require.NotNil(t, session, "complete must set the session cookie")
	if returnTo != "" {
		assert.Equal(t, returnTo, compRec.Header().Get("Location"))
	}
	return session
}

// authIssuerValidate validates a session token with an issuer built on the same
// secret/clock as the harness (the handler's issuer is not directly exposed).
func (o *oauthHarness) authIssuerValidate(token string) (auth.Principal, error) {
	svc, err := auth.NewService(auth.Config{JWTSecret: testJWTSecret, PasswordEnabled: true}, clock.NewFake(testClockStart))
	require.NoError(o.t, err)
	return svc.Issuer().Validate(token)
}

// completePathFromLocation extracts the /complete path+query from a callback's
// tenant-host redirect Location.
func completePathFromLocation(t *testing.T, location string) string {
	t.Helper()
	require.NotEmpty(t, location)
	u, err := url.Parse(location)
	require.NoError(t, err)
	require.Equal(t, "/api/v1/auth/oauth/complete", u.Path)
	return u.Path + "?" + u.RawQuery
}

// driveGetNoRedirect issues a GET that does NOT follow redirects, so the caller
// can inspect the Location (used to walk the mock authorize hop).
func driveGetNoRedirect(t *testing.T, rawURL string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// queryOf returns the "?..." suffix of a path, or "".
func queryOf(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[i:]
	}
	return ""
}
