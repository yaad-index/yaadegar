package api_test

import (
	"net/http"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oapi-codegen/nullable"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// eventList creates a list with the given event date and a priced item, returning
// the list and item.
func (h *harness) eventList(event *time.Time) (gen.List, gen.Item) {
	h.t.Helper()
	body := gen.ListCreate{Title: "Party"}
	if event != nil {
		body.EventDate = &openapi_types.Date{Time: *event}
	}
	resp, b := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(), body)
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", b)
	list := decode[gen.List](h.t, b)
	item := h.pricedItem(*list.Id, 10000, "EUR")
	return list, item
}

// giverStatuses probes the three giver-surface calls (public view, reserve,
// contribute) on a list/item and returns their status codes.
func (h *harness) giverStatuses(slug, itemID string) (public, reserve, contribute int) {
	h.t.Helper()
	pr, _ := h.req(http.MethodGet, "/public/"+slug, h.ownerHost(), "", nil)
	rr, _ := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations", h.ownerHost(), "",
		map[string]any{"quantity": 1})
	cr, _ := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/contributions", h.ownerHost(), "",
		map[string]any{"pledged": map[string]any{"amount_minor": 10000, "currency": "EUR"}, "contact_email": "g@example.com"})
	return pr.StatusCode, rr.StatusCode, cr.StatusCode
}

func TestEventDatedListLifecycle(t *testing.T) {
	h := newHarness(t)
	// Event date a few days after the harness clock start (2027-06-15).
	event := time.Date(2027, 6, 20, 0, 0, 0, 0, time.UTC)
	list, item := h.eventList(&event)

	t.Run("before the event: live", func(t *testing.T) {
		h.clk.Set(time.Date(2027, 6, 18, 9, 0, 0, 0, time.UTC))
		pub, res, con := h.giverStatuses(*list.ShareSlug, *item.Id)
		assert.Equal(t, http.StatusOK, pub)
		assert.Equal(t, http.StatusCreated, res)
		// Reserve above consumed the single unit; contribute now conflicts (409),
		// which still proves the list wasn't gated (not 410).
		assert.NotEqual(t, http.StatusGone, con)
	})

	t.Run("on the event day: still live", func(t *testing.T) {
		h2 := newHarness(t)
		l, it := h2.eventList(&event)
		h2.clk.Set(time.Date(2027, 6, 20, 23, 0, 0, 0, time.UTC))
		pub, res, _ := h2.giverStatuses(*l.ShareSlug, *it.Id)
		assert.Equal(t, http.StatusOK, pub, "the event day itself is live")
		assert.Equal(t, http.StatusCreated, res)
	})

	t.Run("after the event: all giver calls are Gone", func(t *testing.T) {
		h3 := newHarness(t)
		l, it := h3.eventList(&event)
		h3.clk.Set(time.Date(2027, 6, 21, 0, 1, 0, 0, time.UTC))
		pub, res, con := h3.giverStatuses(*l.ShareSlug, *it.Id)
		assert.Equal(t, http.StatusGone, pub)
		assert.Equal(t, http.StatusGone, res)
		assert.Equal(t, http.StatusGone, con)

		// The owner surface is unaffected: still readable and patchable.
		resp, _ := h3.req(http.MethodGet, "/api/v1/lists/"+*l.Id, h3.ownerHost(), h3.ownerToken(), nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Clearing the event date brings the list back to life.
		resp, _ = h3.req(http.MethodPatch, "/api/v1/lists/"+*l.Id, h3.ownerHost(), h3.ownerToken(),
			gen.ListUpdate{Active: ptr(true), EventDate: nullable.NewNullableWithValue(openapi_types.Date{Time: time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC)})})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		pub, _, _ = h3.giverStatuses(*l.ShareSlug, *it.Id)
		assert.Equal(t, http.StatusOK, pub, "extending the date reactivates the giver surface")
	})
}

func TestOpenEndedListNeverDisables(t *testing.T) {
	h := newHarness(t)
	list, item := h.eventList(nil) // no event date

	// Far in the future, an open-ended list is still live.
	h.clk.Set(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	pub, res, _ := h.giverStatuses(*list.ShareSlug, *item.Id)
	assert.Equal(t, http.StatusOK, pub)
	assert.Equal(t, http.StatusCreated, res)
}
