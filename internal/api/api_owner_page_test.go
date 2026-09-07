package api_test

import (
	"net/http"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// --- helpers ---

// newList creates a list for the seeded owner and returns it.
func (h *harness) newList(title string) gen.List {
	h.t.Helper()
	resp, b := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: title})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", b)
	return decode[gen.List](h.t, b)
}

// setVisibility patches a list's visibility, the flag that governs whether it is
// listed on the owner page.
func (h *harness) setVisibility(listID string, v gen.ListVisibility) {
	h.t.Helper()
	resp, b := h.req(http.MethodPatch, "/api/v1/lists/"+listID, h.ownerHost(), h.ownerToken(),
		gen.ListUpdate{Visibility: &v})
	require.Equal(h.t, http.StatusOK, resp.StatusCode, "body: %s", b)
}

// createOwnerKey mints the seeded owner's key and returns it.
func (h *harness) createOwnerKey() string {
	h.t.Helper()
	resp, b := h.req(http.MethodPost, "/api/v1/me/owner-key", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(h.t, http.StatusOK, resp.StatusCode, "body: %s", b)
	k := decode[gen.OwnerKey](h.t, b)
	require.NotNil(h.t, k.OwnerKey)
	return *k.OwnerKey
}

// ownerPage fetches the public owner page, returning the raw response and body. The
// raw bytes matter: some assertions here are about what is absent from the JSON, and
// a decoded struct can only show the fields the schema defines.
func (h *harness) ownerPage(key string) (*http.Response, []byte) {
	h.t.Helper()
	return h.req(http.MethodGet, "/public/owners/"+key, h.ownerHost(), "", nil)
}

// --- key lifecycle ---

func TestOwnerKeyIsAbsentUntilExplicitlyCreated(t *testing.T) {
	h := newHarness(t)

	resp, b := h.req(http.MethodGet, "/api/v1/me/owner-key", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", b)
	assert.Nil(t, decode[gen.OwnerKey](t, b).OwnerKey,
		"an account has no owner page until its owner asks for one")

	key := h.createOwnerKey()
	assert.NotEmpty(t, key)

	resp, b = h.req(http.MethodGet, "/api/v1/me/owner-key", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got := decode[gen.OwnerKey](t, b)
	require.NotNil(t, got.OwnerKey)
	assert.Equal(t, key, *got.OwnerKey, "reading the key does not change it")
}

func TestOwnerKeyRotationRevokesThePreviousKey(t *testing.T) {
	h := newHarness(t)
	list := h.newList("Birthday")
	h.setVisibility(*list.Id, gen.Public)

	first := h.createOwnerKey()
	resp, _ := h.ownerPage(first)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	second := h.createOwnerKey()
	require.NotEqual(t, first, second, "rotating mints a different key")

	resp, _ = h.ownerPage(first)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"the old link stops resolving — rotation is revocation")

	resp, _ = h.ownerPage(second)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the new link serves the same lists")
}

func TestOwnerKeyRequiresAuthentication(t *testing.T) {
	h := newHarness(t)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		resp, _ := h.req(method, "/api/v1/me/owner-key", h.ownerHost(), "", nil)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"%s owner-key without a token", method)
	}
}

// --- the page itself ---

func TestOwnerPageCarriesOnlyListedLists(t *testing.T) {
	h := newHarness(t)
	listed := h.newList("Shown")
	unlisted := h.newList("Hidden")
	private := h.newList("AlsoHidden")
	h.setVisibility(*listed.Id, gen.Public)
	h.setVisibility(*unlisted.Id, gen.Unlisted)
	h.setVisibility(*private.Id, gen.Private)

	resp, body := h.ownerPage(h.createOwnerKey())
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	page := decode[gen.PublicOwner](t, body)
	require.NotNil(t, page.Lists)
	require.Len(t, *page.Lists, 1)
	assert.Equal(t, "Shown", *(*page.Lists)[0].Title)
	assert.Equal(t, *listed.ShareSlug, *(*page.Lists)[0].ShareSlug,
		"a listed row carries its slug so it can link through")

	// The property is that an unlisted list's slug never reaches the client at all,
	// not merely that it is absent from the rows we chose to render. Asserting on the
	// raw bytes is what makes this fail if the filter ever moves out of the query.
	assert.NotContains(t, string(body), *unlisted.ShareSlug,
		"an unlisted list's share_slug must not appear anywhere in the response")
	assert.NotContains(t, string(body), *private.ShareSlug)
	assert.NotContains(t, string(body), "Hidden", "nor its title")
}

func TestOwnerPageIsEmptyRatherThanMissingWhenNothingIsListed(t *testing.T) {
	h := newHarness(t)
	h.newList("Kept back") // exists, never listed

	resp, body := h.ownerPage(h.createOwnerKey())
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"a real key with nothing listed is an empty page, not a missing one")
	page := decode[gen.PublicOwner](t, body)
	require.NotNil(t, page.Lists)
	assert.Empty(t, *page.Lists)
}

func TestOwnerPageRejectsUnknownAndEmptyKeys(t *testing.T) {
	h := newHarness(t)
	h.createOwnerKey() // a real key exists in the tenant

	resp, _ := h.ownerPage("nosuchkey")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// A blank-but-present key reaches the handler and is refused there.
	resp, _ = h.ownerPage("%20")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// A literally empty segment is refused by the router before any handler runs, so
	// this asserts routing and nothing about the lookup. Deleting the empty-key guard
	// in ByOwnerKey leaves it passing — the guard's own coverage is
	// TestByOwnerKeyNeverMatchesTheEmptyKey, which exercises it directly. Kept
	// because the route's shape is worth pinning, labelled so it is not mistaken for
	// coverage of the sentinel.
	resp, _ = h.req(http.MethodGet, "/public/owners/", h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOwnerPageOmitsListsTheirOwnLinkWouldRefuse(t *testing.T) {
	event := time.Date(2027, 6, 20, 0, 0, 0, 0, time.UTC)

	t.Run("past its event date", func(t *testing.T) {
		h := newHarness(t)
		resp, b := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
			gen.ListCreate{Title: "Party", EventDate: &openapi_types.Date{Time: event}})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", b)
		list := decode[gen.List](t, b)
		h.setVisibility(*list.Id, gen.Public)
		key := h.createOwnerKey()

		h.clk.Set(time.Date(2027, 6, 18, 9, 0, 0, 0, time.UTC))
		_, body := h.ownerPage(key)
		require.Len(t, *decode[gen.PublicOwner](t, body).Lists, 1, "listed while live")

		h.clk.Set(time.Date(2027, 6, 21, 0, 1, 0, 0, time.UTC))
		resp, body = h.ownerPage(key)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, *decode[gen.PublicOwner](t, body).Lists,
			"a list whose share link answers 410 is not advertised on the index")
		assert.NotContains(t, string(body), *list.ShareSlug)
	})

	t.Run("deactivated", func(t *testing.T) {
		h := newHarness(t)
		list := h.newList("Closed")
		h.setVisibility(*list.Id, gen.Public)
		key := h.createOwnerKey()

		_, body := h.ownerPage(key)
		require.Len(t, *decode[gen.PublicOwner](t, body).Lists, 1)

		resp, b := h.req(http.MethodPatch, "/api/v1/lists/"+*list.Id, h.ownerHost(), h.ownerToken(),
			gen.ListUpdate{Active: ptr(false)})
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", b)

		_, body = h.ownerPage(key)
		assert.Empty(t, *decode[gen.PublicOwner](t, body).Lists)
	})
}

func TestOwnerPageWithholdsADisplayNameThatIsJustTheEmail(t *testing.T) {
	h := newHarness(t)
	// seedNamedUser reproduces how accounts are really created: the display name
	// defaults to the account email (#185), so users.name holds a literal address for
	// anyone who never chose a name.
	const addr = "wren@example.invalid"
	h.seedNamedUser("wren", addr, "pw-wren")
	token := h.login("wren", "pw-wren")

	resp, b := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), token,
		gen.ListCreate{Title: "Shown"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", b)
	list := decode[gen.List](t, b)
	resp, b = h.req(http.MethodPatch, "/api/v1/lists/"+*list.Id, h.ownerHost(), token,
		gen.ListUpdate{Visibility: ptr(gen.Public)})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", b)

	resp, b = h.req(http.MethodPost, "/api/v1/me/owner-key", h.ownerHost(), token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", b)
	key := *decode[gen.OwnerKey](t, b).OwnerKey

	_, body := h.ownerPage(key)
	require.Len(t, *decode[gen.PublicOwner](t, body).Lists, 1, "precondition: the page serves")
	assert.Nil(t, decode[gen.PublicOwner](t, body).DisplayName,
		"no heading rather than an email address")
	assert.NotContains(t, string(body), addr,
		"the address must not reach anyone holding the key")

	resp, b = h.req(http.MethodPut, "/api/v1/me/profile", h.ownerHost(), token,
		gen.UpdateProfileRequest{Name: "Wren"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", b)

	_, body = h.ownerPage(key)
	got := decode[gen.PublicOwner](t, body).DisplayName
	require.NotNil(t, got, "a chosen name is the page heading")
	assert.Equal(t, "Wren", *got)
	assert.NotContains(t, string(body), addr, "and the address still does not appear")
}

func TestOwnerPageRowsCarryCountsAndPreviews(t *testing.T) {
	h := newHarness(t)
	list := h.newList("Gifts")
	h.pricedItem(*list.Id, 10000, "EUR")
	h.pricedItem(*list.Id, 20000, "EUR")
	h.setVisibility(*list.Id, gen.Public)

	_, body := h.ownerPage(h.createOwnerKey())
	rows := *decode[gen.PublicOwner](t, body).Lists
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].ItemCount)
	assert.Equal(t, 2, *rows[0].ItemCount)
	require.NotNil(t, rows[0].ItemPreviews)
	assert.Len(t, *rows[0].ItemPreviews, 2, "the preview cluster is populated on this read")
}

func TestOwnerPageIsReadOnlyAndTenantScoped(t *testing.T) {
	h := newHarness(t)
	list := h.newList("Shown")
	h.setVisibility(*list.Id, gen.Public)
	key := h.createOwnerKey()

	// No write verb is served here; givers act through each list's own surface.
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		resp, _ := h.req(method, "/public/owners/"+key, h.ownerHost(), "", nil)
		assert.NotContains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode,
			"%s must not be served", method)
	}

	// A key from one tenant does not resolve on another tenant's host.
	other := h.seedOwnerTenant("bob")
	resp, _ := h.req(http.MethodGet, "/public/owners/"+key, other.host, "", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"the owner page resolves within the Host-resolved tenant, like every other public read")
}

// --- storage-level: the filter is in the query ---

func TestListedByOwnerSelectsOnlyListedLists(t *testing.T) {
	h := newHarness(t)
	ts := h.store.ForTenant(h.tenant)

	mk := func(title string, v storage.Visibility) storage.List {
		l, err := ts.Lists().Create(t.Context(),
			storage.List{Title: title, Visibility: v, Active: true}, h.owner.ID)
		require.NoError(t, err)
		return l
	}
	listed := mk("Shown", storage.VisibilityPublic)
	mk("Hidden", storage.VisibilityUnlisted)
	mk("AlsoHidden", storage.VisibilityPrivate)

	got, err := ts.Lists().ListedByOwner(t.Context(), h.owner.ID, storage.Page{Limit: 50})
	require.NoError(t, err)
	require.Len(t, got, 1, "the predicate is in the query, so unlisted rows are never read")
	assert.Equal(t, listed.ID, got[0].ID)
}

func TestByOwnerKeyNeverMatchesTheEmptyKey(t *testing.T) {
	h := newHarness(t)
	ts := h.store.ForTenant(h.tenant)

	// The seeded owner has no key, so their stored value is the "" sentinel.
	stored, err := ts.Users().Get(t.Context(), h.owner.ID)
	require.NoError(t, err)
	require.Empty(t, stored.OwnerKey, "precondition: no key minted")

	_, err = ts.Users().ByOwnerKey(t.Context(), "")
	assert.ErrorIs(t, err, storage.ErrNotFound,
		"the empty key must not resolve to an account that never published")

	key, err := ts.Users().RotateOwnerKey(t.Context(), h.owner.ID)
	require.NoError(t, err)
	found, err := ts.Users().ByOwnerKey(t.Context(), key)
	require.NoError(t, err)
	assert.Equal(t, h.owner.ID, found.ID)
}

func TestRotateOwnerKeyReplacesRatherThanAccumulates(t *testing.T) {
	h := newHarness(t)
	ts := h.store.ForTenant(h.tenant)

	first, err := ts.Users().RotateOwnerKey(t.Context(), h.owner.ID)
	require.NoError(t, err)
	second, err := ts.Users().RotateOwnerKey(t.Context(), h.owner.ID)
	require.NoError(t, err)
	require.NotEqual(t, first, second)

	_, err = ts.Users().ByOwnerKey(t.Context(), first)
	assert.ErrorIs(t, err, storage.ErrNotFound, "the superseded key is gone, not merely shadowed")

	found, err := ts.Users().ByOwnerKey(t.Context(), second)
	require.NoError(t, err)
	assert.Equal(t, h.owner.ID, found.ID)
}
