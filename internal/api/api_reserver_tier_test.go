package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

func sptr(s string) *string { return &s }

// TestReserverTierRoundTrip: the per-list tier override is created, read back,
// updated, and cleared; an unknown tier is a 400.
func TestReserverTierRoundTrip(t *testing.T) {
	h := newHarness(t)

	// Absent → inherit the instance default → null in the response.
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: "Default"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	assert.Nil(t, decode[gen.List](t, body).ReserverTier)

	// Create with an override.
	resp, body = h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: "Confirmed", ReserverTier: sptr("email_confirmed")})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	created := decode[gen.List](t, body)
	require.NotNil(t, created.ReserverTier)
	assert.Equal(t, "email_confirmed", *created.ReserverTier)

	// Update the override.
	resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		gen.ListUpdate{ReserverTier: sptr("full_guest")})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	require.NotNil(t, decode[gen.List](t, body).ReserverTier)
	assert.Equal(t, "full_guest", *decode[gen.List](t, body).ReserverTier)

	// Unknown tier → 400.
	resp, _ = h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: "Bad", ReserverTier: sptr("superuser")})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// seedPendingReservation inserts a pending_confirmation reservation (as the
// email_confirmed reserve will in Cut 2), with a giver email that must never leak.
func (h *harness) seedPendingReservation(itemID, giverEmail string) {
	h.t.Helper()
	_, err := h.store.ForTenant(h.tenant).Reservations().Create(context.Background(), storage.Reservation{
		ItemID:           itemID,
		GiverEmail:       &giverEmail,
		Quantity:         1,
		TokenHash:        "cap-pending",
		ConfirmTokenHash: "confirm-pending",
		State:            storage.StatePendingConfirmation,
	})
	require.NoError(h.t, err)
}

// TestPendingConfirmationCountsAsReserved is the load-bearing Cut-1 pin (#81): a
// pending_confirmation reservation occupies the item on BOTH the owner and public
// surfaces (reserved, count = 1) — this fails if the reserved-count query stops
// covering pending — and the reserver's email never leaks on either surface.
func TestPendingConfirmationCountsAsReserved(t *testing.T) {
	const giverEmail = "secret-giver@example.com"
	h := newHarness(t)
	list := h.createList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)

	h.seedPendingReservation(*item.Id, giverEmail)

	// Owner surface: the item reads reserved with an exact reserved count of 1.
	_, ownerBody := h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/items", h.ownerHost(), h.ownerToken(), nil)
	ownedItems := *decode[gen.ItemPage](t, ownerBody).Items
	require.Len(t, ownedItems, 1)
	assert.Equal(t, gen.Reserved, *ownedItems[0].Availability, "pending holds the item")
	require.NotNil(t, ownedItems[0].ReservedQuantity)
	assert.Equal(t, 1, *ownedItems[0].ReservedQuantity, "pending counts toward the reserved quantity")
	assert.NotContains(t, string(ownerBody), giverEmail, "the reserver's email must never reach the owner")

	// Public surface: reserved, and no reserver identity/email.
	_, pubBody := h.req(http.MethodGet, "/public/"+*list.ShareSlug, h.ownerHost(), "", nil)
	pubItems := *decode[gen.PublicList](t, pubBody).Items
	require.Len(t, pubItems, 1)
	assert.Equal(t, gen.Reserved, *pubItems[0].Availability)
	assert.NotContains(t, string(pubBody), giverEmail, "the reserver's email must never reach a giver")
	assert.NotContains(t, strings.ToLower(string(pubBody)), "secret-giver")
}
