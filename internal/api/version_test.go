package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// GET /api/v1/version is the instance-level, unauthenticated build-version surface
// (ADR-0014 §3). These assert the three properties that let a monitoring poll reach
// it through the web edge's /api/v1/ passthrough: it needs no tenant host, it needs
// no auth token, and it reports the running stamp as JSON.

func TestVersion_ReportsConfiguredVersion(t *testing.T) {
	h := newHarnessVersion(t, "9.9.9-test")
	// Any host, no configured tenant, no bearer token — the version does not depend
	// on the tenant, so a monitor must not need one.
	resp, body := h.req(http.MethodGet, "/api/v1/version", "anything.invalid", "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	got := decode[gen.VersionInfo](t, body)
	assert.Equal(t, "9.9.9-test", got.Version)
}

func TestVersion_EmptyStampReportsUnknown(t *testing.T) {
	// A server built without a stamp reports "unknown", not a blank — the same
	// fail-distinct contract as the version subcommand (#225), so a monitor never
	// sees an empty version it might read as "matched".
	h := newHarness(t)
	resp, body := h.req(http.MethodGet, "/api/v1/version", "anything.invalid", "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	got := decode[gen.VersionInfo](t, body)
	assert.Equal(t, "unknown", got.Version)
}

func TestVersion_UnknownHostStillAnswers(t *testing.T) {
	// A path that DOES resolve a tenant 404s an unknown host (see TestUnknownHostIs404);
	// /api/v1/version must NOT — it skips tenant resolution, so a host with no configured
	// tenant still gets the version rather than a 404.
	h := newHarnessVersion(t, "1.2.3")
	resp, _ := h.req(http.MethodGet, "/api/v1/version", "nobody."+baseDomain, "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
