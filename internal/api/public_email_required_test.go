package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// TestPublicListTierFlags: the public list response advertises whether a giver email
// (#144) or a signed-in account (#170) is required to reserve, each derived from the
// effective reserver tier, so the giver UI can adapt up front instead of failing at
// submit. At most one flag is true.
func TestPublicListTierFlags(t *testing.T) {
	h := newHarness(t)

	newList := func(title string, tier *string) gen.List {
		_, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
			gen.ListCreate{Title: title, ReserverTier: tier})
		return decode[gen.List](t, body)
	}
	publicList := func(slug string) gen.PublicList {
		_, body := h.req(http.MethodGet, "/public/"+slug, h.ownerHost(), "", nil)
		return decode[gen.PublicList](t, body)
	}

	// email_confirmed tier → email required, account not.
	confirm := publicList(*newList("Confirm", sptr("email_confirmed")).ShareSlug)
	require.NotNil(t, confirm.EmailRequired)
	require.NotNil(t, confirm.AccountRequired)
	assert.True(t, *confirm.EmailRequired)
	assert.False(t, *confirm.AccountRequired)

	// registered tier → account required, email not (the reserve is account-bound).
	registered := publicList(*newList("Registered", sptr("registered")).ShareSlug)
	require.NotNil(t, registered.AccountRequired)
	require.NotNil(t, registered.EmailRequired)
	assert.True(t, *registered.AccountRequired)
	assert.False(t, *registered.EmailRequired)

	// full_guest tier → neither flag set.
	guest := publicList(*newList("Guest", sptr("full_guest")).ShareSlug)
	require.NotNil(t, guest.EmailRequired)
	require.NotNil(t, guest.AccountRequired)
	assert.False(t, *guest.EmailRequired)
	assert.False(t, *guest.AccountRequired)
}
