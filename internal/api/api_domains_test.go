package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// otherTenant is a second tenant with an owner, for cross-tenant custom-domain
// tests. Token mints a fresh owner JWT at the current fake-clock time — call it
// after advancing h.clk so the token isn't already expired.
type otherTenant struct {
	h       *harness
	host    string
	ownerID string
	tenID   string
}

func (h *harness) seedOwnerTenant(subdomain string) otherTenant {
	h.t.Helper()
	ctx := context.Background()
	ten, err := h.store.CreateTenant(ctx, storage.Tenant{Subdomain: subdomain})
	require.NoError(h.t, err)
	owner, err := h.store.ForTenant(ten).Users().Create(ctx, storage.User{Name: subdomain})
	require.NoError(h.t, err)
	return otherTenant{h: h, host: subdomain + "." + baseDomain, ownerID: owner.ID, tenID: ten.ID}
}

func (o otherTenant) token() string { return o.h.tokenFor(o.ownerID, o.tenID) }

func (h *harness) tryAddDomain(host, token, hostname string) *http.Response {
	h.t.Helper()
	resp, _ := h.req(http.MethodPost, "/api/v1/domains", host, token, map[string]any{"hostname": hostname})
	return resp
}

// TestAddDomainReclaimsExpiredUnverified is the #42 guard: an unverified claim past
// the TTL frees its hostname for another tenant, but blocks within the window.
func TestAddDomainReclaimsExpiredUnverified(t *testing.T) {
	h := newHarness(t)
	const host = "squat.example.com"

	alice := h.addDomain(host) // Alice parks it but never verifies.
	bob := h.seedOwnerTenant("bob")

	// Within the window, Bob still cannot take it.
	assert.Equal(t, http.StatusConflict, h.tryAddDomain(bob.host, bob.token(), host).StatusCode,
		"an unverified claim still blocks within the window")

	// Past the window, Bob reclaims it.
	h.clk.Advance(testDomainClaimTTL + time.Hour)
	resp, body := h.req(http.MethodPost, "/api/v1/domains", bob.host, bob.token(), map[string]any{"hostname": host})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	assert.NotEqual(t, *alice.Id, *decode[gen.Domain](t, body).Id, "a fresh claim, not the stale row")

	// Alice's stale claim is gone.
	_, lb := h.req(http.MethodGet, "/api/v1/domains", h.ownerHost(), h.ownerToken(), nil)
	assert.Empty(t, decode[[]gen.Domain](t, lb), "the reclaimed hostname left the original tenant")
}

// TestAddDomainVerifiedNeverReclaimed pins that a verified domain never expires by
// this path, no matter how far past the window.
func TestAddDomainVerifiedNeverReclaimed(t *testing.T) {
	h := newHarness(t)
	const host = "keep.example.com"

	d := h.addDomain(host)
	h.resolver.txt["_yaadegar-verify."+host] = []string{*d.VerificationToken}
	resp, body := h.req(http.MethodPost, "/api/v1/domains/"+*d.Id+"/verify", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, *decode[gen.Domain](t, body).Verified, "domain is verified before the reclaim attempt")

	bob := h.seedOwnerTenant("bob")
	h.clk.Advance(365 * 24 * time.Hour)
	assert.Equal(t, http.StatusConflict, h.tryAddDomain(bob.host, bob.token(), host).StatusCode,
		"a verified domain is never reclaimed")
}

// TestAddDomainConcurrentReclaimSingleWinner: two tenants race to reclaim the same
// expired hostname at once; exactly one wins (the reclaim + insert is atomic).
func TestAddDomainConcurrentReclaimSingleWinner(t *testing.T) {
	h := newHarness(t)
	const host = "race.example.com"
	h.addDomain(host) // an unverified claim by the seeded owner
	bravo := h.seedOwnerTenant("bravo")
	charlie := h.seedOwnerTenant("charlie")
	h.clk.Advance(testDomainClaimTTL + time.Hour) // now expired; mint tokens after.
	racers := [][2]string{{bravo.host, bravo.token()}, {charlie.host, charlie.token()}}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
		start   = make(chan struct{})
	)
	for _, who := range racers {
		wg.Add(1)
		go func(host, token string) {
			defer wg.Done()
			<-start
			if h.tryAddDomain(host, token, "race.example.com").StatusCode == http.StatusCreated {
				mu.Lock()
				created++
				mu.Unlock()
			}
		}(who[0], who[1])
	}
	close(start)
	wg.Wait()
	assert.Equal(t, 1, created, "exactly one tenant reclaims the expired hostname")
}

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
