package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// fakeResolver serves DNS TXT records from a map (no real DNS).
type fakeResolver struct {
	txt map[string][]string
	err error
}

func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.txt[name], nil
}

func (h *harness) addDomain(hostname string) gen.Domain {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/domains", h.ownerHost(), h.ownerToken(),
		map[string]any{"hostname": hostname})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	return decode[gen.Domain](h.t, body)
}

func TestAddDomain(t *testing.T) {
	h := newHarness(t)
	d := h.addDomain("gifts.example.com")
	assert.Equal(t, "gifts.example.com", *d.Hostname)
	assert.False(t, *d.Verified, "a new domain is unverified")
	assert.Equal(t, "cname.yaadegar.test", *d.CnameTarget)
	require.NotNil(t, d.VerificationToken)
	assert.NotEmpty(t, *d.VerificationToken)

	// Bad hostname → 400.
	resp, _ := h.req(http.MethodPost, "/api/v1/domains", h.ownerHost(), h.ownerToken(),
		map[string]any{"hostname": "not a hostname"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Duplicate hostname → 409 (first-add-wins).
	resp, _ = h.req(http.MethodPost, "/api/v1/domains", h.ownerHost(), h.ownerToken(),
		map[string]any{"hostname": "gifts.example.com"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestListAndDeleteDomains(t *testing.T) {
	h := newHarness(t)
	d := h.addDomain("shop.example.com")

	resp, body := h.req(http.MethodGet, "/api/v1/domains", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, decode[[]gen.Domain](t, body), 1)

	resp, _ = h.req(http.MethodDelete, "/api/v1/domains/"+*d.Id, h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, _ = h.req(http.MethodDelete, "/api/v1/domains/"+*d.Id, h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestVerifyDomain covers the whole verification flow plus the load-bearing
// invariant: an unverified domain does not resolve; a verified one does.
func TestVerifyDomain(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	d := h.addDomain("gifts.example.com")
	txtName := "_yaadegar-verify.gifts.example.com"

	// Invariant: before verification, the hostname must NOT resolve to the tenant.
	_, err := h.store.TenantByCustomDomain(ctx, "gifts.example.com")
	assert.ErrorIs(t, err, storage.ErrNotFound, "unverified domains must never route")

	// No TXT record yet → verify returns still-unverified (a retry state, not an error).
	resp, body := h.req(http.MethodPost, "/api/v1/domains/"+*d.Id+"/verify", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.False(t, *decode[gen.Domain](t, body).Verified)

	// Publish the matching TXT record, then verify → verified.
	h.resolver.txt[txtName] = []string{"unrelated", *d.VerificationToken}
	resp, body = h.req(http.MethodPost, "/api/v1/domains/"+*d.Id+"/verify", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, *decode[gen.Domain](t, body).Verified)

	// Now the hostname resolves to the tenant.
	ten, err := h.store.TenantByCustomDomain(ctx, "gifts.example.com")
	require.NoError(t, err)
	assert.Equal(t, h.tenant.ID, ten.ID)

	// Idempotent: re-verifying an already-verified domain is a clean success.
	resp, body = h.req(http.MethodPost, "/api/v1/domains/"+*d.Id+"/verify", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, *decode[gen.Domain](t, body).Verified)
}

func TestVerifyDomainDNSErrorIsNotServerError(t *testing.T) {
	h := newHarness(t)
	d := h.addDomain("gifts.example.com")
	h.resolver.err = assertAnError // resolver fails (NXDOMAIN/timeout analog)

	resp, body := h.req(http.MethodPost, "/api/v1/domains/"+*d.Id+"/verify", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "a DNS error is a retry state, not a 500")
	assert.False(t, *decode[gen.Domain](t, body).Verified)
}

var assertAnError = &dnsErr{}

type dnsErr struct{}

func (*dnsErr) Error() string { return "simulated DNS failure" }
