package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/preview"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

const baseDomain = "example.test"

// testJWTSecret is a fixed >=32-byte signing secret for the test auth service.
const testJWTSecret = "test-jwt-secret-of-sufficient-length-0123456789"

// testDomainClaimTTL is the harness's unverified custom-domain claim window; tests
// move h.clk past it to exercise reclaiming (#42).
const testDomainClaimTTL = 7 * 24 * time.Hour

// testClockStart is the fake "now" every harness starts at; time-gated tests move
// it via h.clk.
var testClockStart = time.Date(2027, 6, 15, 12, 0, 0, 0, time.UTC)

type harness struct {
	t        *testing.T
	h        http.Handler
	store    storage.Store
	tenant   storage.Tenant
	owner    storage.User
	email    *email.FakeSender
	clk      *clock.Fake
	preview  *preview.FakeFetcher
	resolver *fakeResolver
	authSvc  *auth.Service
}

func newHarness(t *testing.T) *harness { return newHarnessBuild(t, nil, false) }

// newHarnessLimited injects a login rate limiter (nil → no limiting).
func newHarnessLimited(t *testing.T, limiter auth.Limiter) *harness {
	return newHarnessBuild(t, limiter, false)
}

// newHarnessTrusted builds a harness with X-Forwarded-Host trust enabled.
func newHarnessTrusted(t *testing.T) *harness {
	return newHarnessBuild(t, nil, true)
}

func newHarnessBuild(t *testing.T, limiter auth.Limiter, trustForwardedHost bool) *harness {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "api.db")
	store, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(ctx))
	t.Cleanup(func() { _ = store.Close() })

	tenant, err := store.CreateTenant(ctx, storage.Tenant{Subdomain: "alice"})
	require.NoError(t, err)
	owner, err := store.ForTenant(tenant).Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)

	fake := &email.FakeSender{}
	clk := clock.NewFake(testClockStart)
	pf := &preview.FakeFetcher{} // hermetic: no real network in API tests
	fr := &fakeResolver{txt: map[string][]string{}}
	authSvc, err := auth.NewService(auth.Config{JWTSecret: testJWTSecret, PasswordEnabled: true}, clk)
	require.NoError(t, err)
	h := api.NewHandler(store, api.Options{
		BaseDomain:         baseDomain,
		Logger:             slog.New(slog.DiscardHandler),
		Email:              fake,
		Clock:              clk,
		Previewer:          preview.New(pf),
		Resolver:           fr,
		Auth:               authSvc,
		TrustForwardedHost: trustForwardedHost,
		LoginLimiter:       limiter,
		DomainCNAMETarget:  "cname.yaadegar.test",
		DomainClaimTTL:     testDomainClaimTTL,
	})
	return &harness{t: t, h: h, store: store, tenant: tenant, owner: owner, email: fake, clk: clk, preview: pf, resolver: fr, authSvc: authSvc}
}

// reqH issues a request through the handler with arbitrary headers. host sets
// the Host header (tenant routing).
func (h *harness) reqH(method, path, host string, headers map[string]string, body any) (*http.Response, []byte) {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(h.t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://"+host+path, r)
	require.NoError(h.t, err)
	req.Host = host
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return rec.result(), rec.body.Bytes()
}

// req issues a request; token, if non-empty, is sent as a bearer token.
func (h *harness) req(method, path, host, token string, body any) (*http.Response, []byte) {
	h.t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return h.reqH(method, path, host, headers, body)
}

// ownerHost / ownerToken are the defaults for the seeded tenant+owner. ownerToken
// mints a real session JWT for the seeded owner via the test auth service (the
// same fake clock backs issue + validate, so expiry stays consistent).
func (h *harness) ownerHost() string { return "alice." + baseDomain }
func (h *harness) ownerToken() string {
	return h.tokenFor(h.owner.ID, h.tenant.ID)
}

// tokenFor mints an owner JWT for an arbitrary user/tenant, for cross-tenant and
// negative auth tests.
func (h *harness) tokenFor(userID, tenantID string) string {
	tok, err := h.authSvc.Issuer().Issue(auth.Principal{
		UserID: userID, TenantID: tenantID, Role: auth.RoleOwner,
	})
	require.NoError(h.t, err)
	return tok
}

// adminToken mints an ordinary owner session token for an is_admin owner (ADR-0010):
// admin is a capability on an owner account, so the admin surface is reached with a
// normal owner token whose home tenant carries the flag.
func (h *harness) adminToken(u storage.User) string {
	return h.tokenFor(u.ID, u.TenantID)
}

// --- a tiny ResponseWriter recorder (avoids httptest server networking) ---

type responseRecorder struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(b)
}
func (r *responseRecorder) WriteHeader(code int) { r.code = code }
func (r *responseRecorder) result() *http.Response {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return &http.Response{StatusCode: r.code, Header: r.header}
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(body, &v), "body: %s", string(body))
	return v
}

func TestHealthz_NoTenantNoAuth(t *testing.T) {
	h := newHarness(t)
	// Any host, no tenant needed, no auth.
	resp, body := h.req(http.MethodGet, "/healthz", "anything.invalid", "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", strings.TrimSpace(string(body)))
}

func TestUnknownHostIs404(t *testing.T) {
	h := newHarness(t)
	resp, body := h.req(http.MethodGet, "/api/v1/lists", "nobody."+baseDomain, "tok", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	assert.NotEmpty(t, body)
}

func TestOwnerAuth(t *testing.T) {
	h := newHarness(t)

	t.Run("missing token", func(t *testing.T) {
		resp, _ := h.req(http.MethodGet, "/api/v1/lists", h.ownerHost(), "", nil)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("invalid token", func(t *testing.T) {
		resp, _ := h.req(http.MethodGet, "/api/v1/lists", h.ownerHost(), "not-a-user", nil)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("valid token", func(t *testing.T) {
		resp, _ := h.req(http.MethodGet, "/api/v1/lists", h.ownerHost(), h.ownerToken(), nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestGetCurrentUser(t *testing.T) {
	h := newHarness(t)
	resp, body := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	u := decode[gen.User](t, body)
	assert.Equal(t, h.owner.ID, *u.Id)
	require.NotNil(t, u.Tenant)
	assert.Equal(t, "alice", *u.Tenant.Subdomain)
}

func TestListsAndItemsCRUD(t *testing.T) {
	h := newHarness(t)
	host, tok := h.ownerHost(), h.ownerToken()

	// Create a list.
	resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok, gen.ListCreate{Title: "Birthday"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	list := decode[gen.List](t, body)
	assert.Equal(t, "Birthday", *list.Title)
	assert.NotEmpty(t, *list.ShareSlug)
	assert.Equal(t, 0, *list.ItemCount)
	listID := *list.Id

	// Get it.
	resp, body = h.req(http.MethodGet, "/api/v1/lists/"+listID, host, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Birthday", *decode[gen.List](t, body).Title)

	// Add two items.
	for _, name := range []string{"Book", "Headphones"} {
		resp, body = h.req(http.MethodPost, "/api/v1/lists/"+listID+"/items", host, tok, gen.ItemCreate{Name: name})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
		it := decode[gen.Item](t, body)
		assert.Equal(t, gen.Available, *it.Availability)
	}

	// List items.
	resp, body = h.req(http.MethodGet, "/api/v1/lists/"+listID+"/items", host, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := decode[gen.ItemPage](t, body)
	assert.Equal(t, 2, *page.Total)
	require.Len(t, *page.Items, 2)

	// item_count reflects the two items now.
	resp, body = h.req(http.MethodGet, "/api/v1/lists/"+listID, host, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, *decode[gen.List](t, body).ItemCount)

	// Update the list.
	resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+listID, host, tok, gen.ListUpdate{Title: ptr("Birthday 2027")})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "Birthday 2027", *decode[gen.List](t, body).Title)

	// Update an item (price).
	itemID := (*page.Items)[0].Id
	resp, body = h.req(http.MethodPatch, "/api/v1/items/"+*itemID, host, tok,
		gen.ItemUpdate{Price: &gen.Money{AmountMinor: 1999, Currency: "EUR"}})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, 1999, decode[gen.Item](t, body).Price.AmountMinor)

	// Delete the item, then the list.
	resp, _ = h.req(http.MethodDelete, "/api/v1/items/"+*itemID, host, tok, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp, _ = h.req(http.MethodDelete, "/api/v1/lists/"+listID, host, tok, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Gone now.
	resp, _ = h.req(http.MethodGet, "/api/v1/lists/"+listID, host, tok, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func ptr[T any](v T) *T { return &v }
