package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

func TestCreateListDecayInheritsByDefault(t *testing.T) {
	h := newHarness(t)
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(), gen.ListCreate{Title: "Open-ended"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	// Absent decay_days → inherit the instance default → null in the response.
	assert.Nil(t, decode[gen.List](t, body).DecayDays)
}

// seedDecayReservation inserts a reservation in a decay state with known keep and
// release tokens.
func (h *harness) seedDecayReservation(itemID string, state storage.ReservationState, keepRaw, releaseRaw string) {
	h.t.Helper()
	_, err := h.store.ForTenant(h.tenant).Reservations().Create(context.Background(), storage.Reservation{
		ItemID:                itemID,
		Quantity:              1,
		TokenHash:             "cap-" + releaseRaw,
		State:                 state,
		DecayReleaseTokenHash: token.Hash(releaseRaw),
		DecayKeepTokenHash:    token.Hash(keepRaw),
	})
	require.NoError(h.t, err)
}

func TestReleaseByDecayToken(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	h.seedDecayReservation(*item.Id, storage.StateReserverNotified, "keep-tok", "rel-tok")

	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))

	// Bad token → 404.
	resp, _ := h.req(http.MethodPost, "/public/decay-release", h.ownerHost(), "", map[string]any{"token": "wrong"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// The release token frees the item → 204.
	resp, _ = h.req(http.MethodPost, "/public/decay-release", h.ownerHost(), "", map[string]any{"token": "rel-tok"})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))

	// Single-use — the reservation is gone.
	resp, _ = h.req(http.MethodPost, "/public/decay-release", h.ownerHost(), "", map[string]any{"token": "rel-tok"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestKeepByDecayToken(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	h.seedDecayReservation(*item.Id, storage.StateReserverNotified, "keep-tok", "rel-tok")

	// Keep renews → 204, item stays reserved (not released).
	resp, _ := h.req(http.MethodPost, "/public/decay-keep", h.ownerHost(), "", map[string]any{"token": "keep-tok"})
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))

	// Renewing cleared both tokens: the keep token is now dead, and so is release.
	resp, _ = h.req(http.MethodPost, "/public/decay-keep", h.ownerHost(), "", map[string]any{"token": "keep-tok"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp, _ = h.req(http.MethodPost, "/public/decay-release", h.ownerHost(), "", map[string]any{"token": "rel-tok"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDecayTokensExpired410(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	// Already auto-expired: both links are dead (410, hashes retained).
	h.seedDecayReservation(*item.Id, storage.StateExpired, "keep-tok", "rel-tok")

	resp, _ := h.req(http.MethodPost, "/public/decay-release", h.ownerHost(), "", map[string]any{"token": "rel-tok"})
	assert.Equal(t, http.StatusGone, resp.StatusCode)
	resp, _ = h.req(http.MethodPost, "/public/decay-keep", h.ownerHost(), "", map[string]any{"token": "keep-tok"})
	assert.Equal(t, http.StatusGone, resp.StatusCode)
}
