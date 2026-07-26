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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

const baseDomain = "example.test"

type harness struct {
	t      *testing.T
	h      http.Handler
	store  storage.Store
	tenant storage.Tenant
	owner  storage.User
}

func newHarness(t *testing.T) *harness {
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

	h := api.NewHandler(store, api.Options{BaseDomain: baseDomain, Logger: slog.New(slog.DiscardHandler)})
	return &harness{t: t, h: h, store: store, tenant: tenant, owner: owner}
}

// req issues a request through the handler. host sets the Host header (tenant
// routing); token, if non-empty, is sent as a bearer token.
func (h *harness) req(method, path, host, token string, body any) (*http.Response, []byte) {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return rec.result(), rec.body.Bytes()
}

// ownerHost / ownerToken are the defaults for the seeded tenant+owner.
func (h *harness) ownerHost() string  { return "alice." + baseDomain }
func (h *harness) ownerToken() string { return h.owner.ID }

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
