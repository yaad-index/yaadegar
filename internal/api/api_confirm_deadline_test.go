package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/decay"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// --- helpers -----------------------------------------------------------------

// confirmList makes an email_confirmed list, optionally overriding the confirm
// window (nil inherits the instance default), and returns it with one item.
func (h *harness) confirmList(title string, windowMinutes *int) (gen.List, gen.Item) {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: title, ReserverTier: sptr("email_confirmed"), ReserverConfirmWindow: windowMinutes})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	list := decode[gen.List](h.t, body)

	resp, body = h.req(http.MethodPost, "/api/v1/lists/"+*list.Id+"/items", h.ownerHost(), h.ownerToken(),
		gen.ItemCreate{Name: "placeholder-item-one"})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	return list, decode[gen.Item](h.t, body)
}

// reservePending reserves one unit on an email_confirmed list and returns the 202
// body. The giver address is required at this tier.
func (h *harness) reservePending(slug, itemID string) gen.ReservationCreated {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": 1, "giver_email": "giver-one@example.invalid"})
	require.Equal(h.t, http.StatusAccepted, resp.StatusCode, "body: %s", body)
	return decode[gen.ReservationCreated](h.t, body)
}

// sweepConfirm runs the real decay sweeper with a confirm window. The window is
// passed in rather than fixed so a test can hand the SAME value to the harness and
// to the sweep, which is how the binary wires it — one config field reaching both.
func (h *harness) sweepConfirm(window time.Duration) {
	h.t.Helper()
	sweeper := decay.NewSweeper(h.store, h.email, h.clk, decay.Config{
		ConfirmWindow: window,
		LinkBase:      "https://alice.example.test",
	}, slog.New(slog.DiscardHandler))
	require.NoError(h.t, sweeper.Sweep(context.Background()))
}

func (h *harness) onlyReservation(itemID string) storage.Reservation {
	h.t.Helper()
	rs, err := h.store.ForTenant(h.tenant).Reservations().ListByItem(context.Background(), itemID)
	require.NoError(h.t, err)
	require.Len(h.t, rs, 1)
	return rs[0]
}

// --- tests -------------------------------------------------------------------

// TestTheConfirmDeadlineIsTheInstantTheSweepWillEnforce is the reason the field is
// an absolute deadline rather than a duration, written as a demonstration instead
// of an argument.
//
// ⚠️ It would be much weaker to assert that the handler and the sweeper call the
// same resolver — that is true of two functions that have both been changed to be
// wrong together. What is asserted here is behavioural: the reservation survives a
// sweep at one nanosecond before the returned instant and does not survive one at
// it. If the handler ever anchors the deadline to anything other than the state_at
// the sweep compares (its own clock, the request time, a second resolution of the
// window), one of these two sweeps changes its answer.
func TestTheConfirmDeadlineIsTheInstantTheSweepWillEnforce(t *testing.T) {
	// One value, handed to the server and to the sweep, exactly as main.go hands one
	// config field to the API and the sweeper.
	const window = 30 * time.Minute

	h := newHarnessConfirmWindow(t, window)
	list, item := h.confirmList("placeholder list", nil)
	created := h.reservePending(*list.ShareSlug, *item.Id)

	require.NotNil(t, created.ConfirmDeadline, "a pending hold on a list that expires must state its deadline")
	deadline := *created.ConfirmDeadline

	// Anchored to the reservation's own state_at — the field the sweep subtracts
	// from — and not to any other now.
	res := h.onlyReservation(*item.Id)
	assert.Equal(t, res.StateAt.Add(window).UTC(), deadline.UTC(),
		"the deadline must be state_at + the resolved window")

	// One nanosecond early: untouched.
	h.clk.Set(deadline.Add(-time.Nanosecond))
	h.sweepConfirm(window)
	assert.Equal(t, storage.StatePendingConfirmation, h.onlyReservation(*item.Id).State,
		"the hold must survive a sweep before the deadline it was given")

	// At the deadline: gone.
	h.clk.Set(deadline)
	h.sweepConfirm(window)
	assert.Equal(t, storage.StateExpired, h.onlyReservation(*item.Id).State,
		"the hold must not survive a sweep at the deadline it was given")
}

// TestTheConfirmDeadlineFollowsTheListOverride pins that the deadline is resolved
// per list and not read off the instance default. The override is deliberately
// SHORTER than the default, so a handler that ignored it would return a later
// instant and the assertion would catch it; an override longer than the default
// would also pass if the code simply added the two.
func TestTheConfirmDeadlineFollowsTheListOverride(t *testing.T) {
	const instanceDefault = 4 * time.Hour
	const overrideMinutes = 15

	h := newHarnessConfirmWindow(t, instanceDefault)
	list, item := h.confirmList("placeholder list", ptr(overrideMinutes))
	created := h.reservePending(*list.ShareSlug, *item.Id)

	require.NotNil(t, created.ConfirmDeadline)
	res := h.onlyReservation(*item.Id)
	assert.Equal(t, res.StateAt.Add(overrideMinutes*time.Minute).UTC(), created.ConfirmDeadline.UTC(),
		"the list's own window must win over the instance default")
}

// TestNoDeadlineIsStatedWhenNothingWillExpire covers the case the issue's wording
// did not: a window of zero DISABLES the confirm-window sweep, so the hold waits
// indefinitely and there is no deadline to state.
//
// ⚠️ Both directions matter and they fail differently. Inheriting a zero instance
// default is the plain case. A list overriding zero ON TOP of a positive default is
// the one that breaks a resolver keyed on the zero value instead of on nil — it
// would silently substitute the default and the giver would be told they must
// confirm by a time at which nothing whatsoever happens.
func TestNoDeadlineIsStatedWhenNothingWillExpire(t *testing.T) {
	t.Run("instance default of zero, inherited", func(t *testing.T) {
		h := newHarnessConfirmWindow(t, 0)
		list, item := h.confirmList("placeholder list", nil)
		assert.Nil(t, h.reservePending(*list.ShareSlug, *item.Id).ConfirmDeadline)
	})

	t.Run("list overrides zero over a positive default", func(t *testing.T) {
		h := newHarnessConfirmWindow(t, 4*time.Hour)
		list, item := h.confirmList("placeholder list", ptr(0))
		assert.Nil(t, h.reservePending(*list.ShareSlug, *item.Id).ConfirmDeadline,
			"an explicit zero override disables expiry and must not fall back to the default")
	})
}

// TestAnImmediateReservationStatesNoDeadline is the control for the whole field: a
// full_guest reservation is active on arrival and never pends, so nothing is ever
// going to release it and the deadline must be absent. Without this, a handler that
// set the field unconditionally would pass every test above.
func TestAnImmediateReservationStatesNoDeadline(t *testing.T) {
	h := newHarnessConfirmWindow(t, 30*time.Minute)
	list := h.createDecayingList("placeholder list", 0)

	resp, body := h.req(http.MethodPost, "/api/v1/lists/"+*list.Id+"/items", h.ownerHost(), h.ownerToken(),
		gen.ItemCreate{Name: "placeholder-item-two"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	item := decode[gen.Item](t, body)

	resp, body = h.req(http.MethodPost, "/public/"+*list.ShareSlug+"/items/"+*item.Id+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": 1, "giver_email": "giver-two@example.invalid"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)

	created := decode[gen.ReservationCreated](t, body)
	require.Equal(t, gen.ReservationCreatedStatusActive, created.Status)
	assert.Nil(t, created.ConfirmDeadline, "an active reservation has no confirm deadline")
}

// lastEmail returns the most recently sent message.
func (h *harness) lastEmail() string {
	h.t.Helper()
	sent := h.email.Sent()
	require.NotEmpty(h.t, sent)
	return sent[len(sent)-1].Body
}

// TestTheConfirmEmailCarriesTheSameDeadlineAsTheResponse puts the deadline on the
// surface the giver actually acts from.
//
// 🔑 The email is the load-bearing surface for this, not the page: the confirm
// happens by following the link, and by then the page that could have shown a
// deadline has been left. A deadline named only on the page is named in the one
// place the giver is no longer looking.
//
// The instant is asserted to be the SAME one the response carries rather than
// merely present, because two surfaces each computing their own deadline is the
// failure this whole change is arranged to avoid.
func TestTheConfirmEmailCarriesTheSameDeadlineAsTheResponse(t *testing.T) {
	const window = 30 * time.Minute

	h := newHarnessConfirmWindow(t, window)
	list, item := h.confirmList("placeholder list", nil)
	created := h.reservePending(*list.ShareSlug, *item.Id)
	require.NotNil(t, created.ConfirmDeadline)

	body := h.lastEmail()
	assert.Contains(t, body, "30 minutes", "the window as someone would say it")
	assert.Contains(t, body, created.ConfirmDeadline.UTC().Format("2006-01-02 15:04 UTC"),
		"the email must name the instant the response returned, not its own")
	assert.Contains(t, body, "/confirm?token=", "the link must survive the addition")
}

// TestTheConfirmEmailSaysNothingAboutADeadlineWhenNothingWillExpire is the omission
// half. A hold on a list with the window disabled is never released, so any sentence
// about running out of time would be false — and it would be false in the direction
// that makes someone hurry for no reason.
func TestTheConfirmEmailSaysNothingAboutADeadlineWhenNothingWillExpire(t *testing.T) {
	h := newHarnessConfirmWindow(t, 0)
	list, item := h.confirmList("placeholder list", nil)
	require.Nil(t, h.reservePending(*list.ShareSlug, *item.Id).ConfirmDeadline)

	body := h.lastEmail()
	assert.NotContains(t, body, "to confirm, until")
	assert.NotContains(t, body, "released")
	assert.Contains(t, body, "/confirm?token=", "the link is still the point of the email")
}

// TestTheConfirmWindowReadsNaturallyAtEveryScale drives the rendering through the
// real email rather than calling the formatter, which keeps it in this package's
// one test-package convention and checks what a giver is actually sent.
//
// ⚠️ The window is configured in MINUTES, so a multi-day list would otherwise be
// described as several thousand of them. The cases that matter are the boundaries
// and the singular, plus a window that divides into neither hours nor days: that one
// stays in minutes deliberately, because the alternative is rounding, and rounding a
// deadline either invents time the sweep will not honour or takes away time it would.
func TestTheConfirmWindowReadsNaturallyAtEveryScale(t *testing.T) {
	h := newHarnessConfirmWindow(t, time.Hour) // never used: every case overrides

	for _, tc := range []struct {
		minutes int
		want    string
	}{
		{1, "1 minute"},
		{45, "45 minutes"},
		{60, "1 hour"},
		{120, "2 hours"},
		{90, "90 minutes"},
		{1440, "1 day"},
		{4320, "3 days"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			list, item := h.confirmList("placeholder list", ptr(tc.minutes))
			h.reservePending(*list.ShareSlug, *item.Id)
			assert.Contains(t, h.lastEmail(), "You have "+tc.want+" to confirm")
		})
	}
}
