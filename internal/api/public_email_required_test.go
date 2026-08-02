package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// TestPublicListEmailRequired: the public list response advertises whether a giver
// email is required (#144), derived from the effective reserver tier, so the giver UI
// can require the field up front instead of failing at submit.
func TestPublicListEmailRequired(t *testing.T) {
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

	// email_confirmed tier → email required.
	confirm := newList("Confirm", sptr("email_confirmed"))
	pub := publicList(*confirm.ShareSlug)
	require.NotNil(t, pub.EmailRequired)
	assert.True(t, *pub.EmailRequired)

	// full_guest tier → email not required.
	guest := newList("Guest", sptr("full_guest"))
	pubGuest := publicList(*guest.ShareSlug)
	require.NotNil(t, pubGuest.EmailRequired)
	assert.False(t, *pubGuest.EmailRequired)
}
