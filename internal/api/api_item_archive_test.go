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

// createDecayingList makes a list whose reservations decay after decayDays, which
// is what lets the archive tests drive a real sweep rather than assert about one.
func (h *harness) createDecayingList(title string, decayDays int) gen.List {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: title, DecayDays: &decayDays})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	return decode[gen.List](h.t, body)
}

// reserveAs reserves one unit as an anonymous giver who leaves an email address.
// The address matters: the decay sweep advances a reservation only when it can
// notify the reserver, so a reservation with no email never leaves `active` and
// would make a "decay did not run" test pass for the wrong reason.
func (h *harness) reserveAs(slug, itemID, giverEmail string) *http.Response {
	h.t.Helper()
	resp, _ := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": 1, "giver_email": giverEmail})
	return resp
}

func (h *harness) archive(itemID string) (*http.Response, gen.ItemArchiveResult) {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/items/"+itemID+"/archive", h.ownerHost(), h.ownerToken(), nil)
	if resp.StatusCode != http.StatusOK {
		return resp, gen.ItemArchiveResult{}
	}
	return resp, decode[gen.ItemArchiveResult](h.t, body)
}

func (h *harness) unarchive(itemID string) *http.Response {
	h.t.Helper()
	resp, _ := h.req(http.MethodDelete, "/api/v1/items/"+itemID+"/archive", h.ownerHost(), h.ownerToken(), nil)
	return resp
}

// publicItemIDs returns the ids a giver can see on the shared list.
func (h *harness) publicItemIDs(slug string) []string {
	h.t.Helper()
	resp, body := h.req(http.MethodGet, "/public/"+slug, h.ownerHost(), "", nil)
	require.Equal(h.t, http.StatusOK, resp.StatusCode, "body: %s", body)
	var ids []string
	for _, it := range *decode[gen.PublicList](h.t, body).Items {
		ids = append(ids, *it.Id)
	}
	return ids
}

// ownerItem reads one item from the owner's own view, where archived items remain
// visible. Fails the test if the owner cannot see it at all.
func (h *harness) ownerItem(listID, itemID string) gen.Item {
	h.t.Helper()
	resp, body := h.req(http.MethodGet, "/api/v1/lists/"+listID+"/items", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(h.t, http.StatusOK, resp.StatusCode)
	for _, it := range *decode[gen.ItemPage](h.t, body).Items {
		if *it.Id == itemID {
			return it
		}
	}
	h.t.Fatalf("owner cannot see item %s", itemID)
	return gen.Item{}
}

// sweepDecay runs the real decay sweeper against the harness's store and clock.
// Nothing is simulated: this is the same Sweeper the binary runs, reading the same
// candidate query the archive predicate was added to.
func (h *harness) sweepDecay() {
	h.t.Helper()
	sweeper := decay.NewSweeper(h.store, h.email, h.clk, decay.Config{
		ResponseWindow: 24 * time.Hour,
		LinkBase:       "https://alice.example.test",
	}, slog.New(slog.DiscardHandler))
	require.NoError(h.t, sweeper.Sweep(context.Background()))
}

func (h *harness) reservationStates(itemID string) []storage.ReservationState {
	h.t.Helper()
	rs, err := h.store.ForTenant(h.tenant).Reservations().ListByItem(context.Background(), itemID)
	require.NoError(h.t, err)
	out := make([]storage.ReservationState, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.State)
	}
	return out
}

// --- the acceptance test -----------------------------------------------------

// TestABoughtItemStaysGoneWhenArchived is the whole point of #419, and it is
// written as a two-item comparison on ONE list swept by ONE sweeper so that the
// only difference between the outcomes is the archive itself.
//
// The bug: "reserved" means state IN ('active','reserver_notified'); decay moves an
// unanswered reservation to 'expired', which drops it out of that set and puts the
// item back on the list. A giver who bought the item and then ignored the reminder
// therefore hands it to a second giver, silently.
//
// The control item is what makes this test able to fail. Asserting only that the
// archived item stayed gone would pass just as well if decay never ran at all —
// a mis-set period, a missing giver email, a sweeper wired to the wrong clock.
// The control item goes through the whole escalation in the same sweeps, so the
// test says "decay ran and did its job here, and was stopped there", not "nothing
// happened".
func TestABoughtItemStaysGoneWhenArchived(t *testing.T) {
	h := newHarness(t)
	list := h.createDecayingList("Birthday", 30)

	bought := h.createItem(*list.Id, "Bought item", 1)
	control := h.createItem(*list.Id, "Control item", 1)

	require.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *bought.Id, "buyer@example.com").StatusCode)
	require.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *control.Id, "other@example.com").StatusCode)

	// The owner archives the item the giver actually bought.
	resp, _ := h.archive(*bought.Id)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Drive decay the whole way: past the 30-day period (so an active reservation
	// is notified) and then past the 24h response window (so a notified one
	// auto-expires). Several sweeps, because each advances a candidate one step.
	h.clk.Advance(31 * 24 * time.Hour)
	h.sweepDecay()
	h.clk.Advance(48 * time.Hour)
	h.sweepDecay()
	h.sweepDecay()

	// The control proves decay really ran: its reservation expired and its item is
	// back on the list, available to a second giver. This is the old behaviour,
	// and on the archived item it is the bug.
	assert.Equal(t, []storage.ReservationState{storage.StateExpired}, h.reservationStates(*control.Id),
		"the control's reservation must have decayed all the way out, or this test proves nothing")
	assert.Contains(t, h.publicItemIDs(*list.ShareSlug), *control.Id,
		"an un-archived item does come back when its reservation decays")

	// The archived item: gone, and staying gone.
	assert.Equal(t, []storage.ReservationState{storage.StateActive}, h.reservationStates(*bought.Id),
		"an archived item's reservation must not advance through decay at all")
	assert.NotContains(t, h.publicItemIDs(*list.ShareSlug), *bought.Id,
		"a bought item that was archived must not return to the list")

	// And the consequence that actually harms someone: a second giver buying the
	// same thing. Refused at the reserve path, not merely hidden from the list.
	assert.Equal(t, http.StatusGone,
		h.reserveAs(*list.ShareSlug, *bought.Id, "second-giver@example.com").StatusCode,
		"a second giver must not be able to reserve an archived item")

	// The giver who bought it is not chased. Without the archive, decay asks them
	// "still planning to buy it?" about a thing they already bought — which is how
	// the reservation ends up released in the first place. They DO get the archive
	// notification (Q1), so this looks only at the reminder, by subject: asserting
	// "no mail to the buyer" would be asserting the opposite of the other decision
	// in this issue.
	const decayReminderSubject = "Still planning to buy your reserved gift?"
	var chased, reminded []string
	for _, m := range h.email.Sent() {
		if m.Subject != decayReminderSubject {
			continue
		}
		reminded = append(reminded, m.To)
		if m.To == "buyer@example.com" {
			chased = append(chased, m.To)
		}
	}
	assert.Empty(t, chased, "an archived item's giver must not be chased by the decay reminder")
	assert.Equal(t, []string{"other@example.com"}, reminded,
		"and the control's giver WAS reminded — so the reminder is running, it is just not running here")

	// The sharpest consequence of stopping decay, and the one a person actually
	// meets: un-archiving has to give the item back in the state it left. If the
	// reservation had decayed away underneath the archive, the item would come back
	// unreserved and a second giver could take it — the same bug, one step later.
	require.Equal(t, http.StatusOK, h.unarchive(*bought.Id).StatusCode)
	assert.Equal(t, []storage.ReservationState{storage.StateActive}, h.reservationStates(*bought.Id),
		"the reservation survived the archive intact")
	assert.Equal(t, http.StatusConflict,
		h.reserveAs(*list.ShareSlug, *bought.Id, "second-giver@example.com").StatusCode,
		"an un-archived item is still held by the giver who reserved it")
}

// TestUnarchivingDoesNotHandTheGiverAnUnearnedExpiry covers the gap between the
// two halves of the fix: the predicate suspends a reservation's CANDIDACY for
// decay, never the wall clock the sweeper measures against. So an item archived
// for longer than the decay period comes back already past it, and without the
// clock restart its giver is chased on the very next sweep for time that passed
// while the item was off the list and they could do nothing about it.
//
// The second half of the test is what stops the fix being "decay is now disabled
// forever on anything that was ever archived": after a fresh full period the
// reservation does decay again.
func TestUnarchivingDoesNotHandTheGiverAnUnearnedExpiry(t *testing.T) {
	h := newHarness(t)
	list := h.createDecayingList("Birthday", 30)
	item := h.createItem(*list.Id, "Item", 1)
	require.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *item.Id, "giver@example.com").StatusCode)

	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))

	// Archived for far longer than the decay period.
	h.clk.Advance(200 * 24 * time.Hour)
	require.Equal(t, http.StatusOK, h.unarchive(*item.Id).StatusCode)

	before := len(h.email.Sent())
	h.sweepDecay()
	assert.Equal(t, []storage.ReservationState{storage.StateActive}, h.reservationStates(*item.Id),
		"the archived interval is not the giver's fault and must not be charged to them")
	assert.Empty(t, h.email.Sent()[before:],
		"and they are not chased about it either")

	// But the clock RESTARTED, it did not stop: a fresh full period still decays.
	h.clk.Advance(31 * 24 * time.Hour)
	h.sweepDecay()
	assert.Equal(t, []storage.ReservationState{storage.StateReserverNotified}, h.reservationStates(*item.Id),
		"un-archiving restarts decay, it does not disable it")
}

// TestUnarchivingRestartsTheClockForAnAlreadyNotifiedReservation is the half that
// a fix touching only last_activity_at would miss. Two of the three decay states
// are driven by state_at, and the reserver_notified path is the worst one to get
// wrong: its expiry sends no email at all, so the giver's hold would vanish in
// silence on the first sweep after the item came back.
func TestUnarchivingRestartsTheClockForAnAlreadyNotifiedReservation(t *testing.T) {
	h := newHarness(t)
	list := h.createDecayingList("Birthday", 30)
	item := h.createItem(*list.Id, "Item", 1)
	require.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *item.Id, "giver@example.com").StatusCode)

	// Drive it to reserver_notified for real, rather than seeding the state.
	h.clk.Advance(31 * 24 * time.Hour)
	h.sweepDecay()
	require.Equal(t, []storage.ReservationState{storage.StateReserverNotified}, h.reservationStates(*item.Id))

	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))
	h.clk.Advance(30 * 24 * time.Hour) // far past the 24h response window
	require.Equal(t, http.StatusOK, h.unarchive(*item.Id).StatusCode)

	h.sweepDecay()
	assert.NotContains(t, h.reservationStates(*item.Id), storage.StateExpired,
		"a notified reservation must not expire in silence for time spent archived")
}

// --- surfaces ----------------------------------------------------------------

func TestArchivedItemLeavesTheGiverSurfaceButNotTheOwnerView(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)

	require.Contains(t, h.publicItemIDs(*list.ShareSlug), *item.Id)
	require.Nil(t, h.ownerItem(*list.Id, *item.Id).ArchivedAt)

	resp, result := h.archive(*item.Id)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, result.Item)
	assert.NotNil(t, result.Item.ArchivedAt, "the archived item reports when it was archived")

	assert.NotContains(t, h.publicItemIDs(*list.ShareSlug), *item.Id)
	assert.NotNil(t, h.ownerItem(*list.Id, *item.Id).ArchivedAt,
		"the owner keeps seeing it, marked archived — otherwise archiving is a one-way door")
}

func TestArchiveIsReversible(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)

	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))
	require.NotContains(t, h.publicItemIDs(*list.ShareSlug), *item.Id)

	require.Equal(t, http.StatusOK, h.unarchive(*item.Id).StatusCode)
	assert.Contains(t, h.publicItemIDs(*list.ShareSlug), *item.Id)
	assert.Nil(t, h.ownerItem(*list.Id, *item.Id).ArchivedAt)
	assert.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *item.Id, "giver@example.com").StatusCode,
		"un-archiving undoes the archive exactly: the item is reservable again")
}

func TestArchivedItemRefusesEveryGivingPath(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))

	assert.Equal(t, http.StatusGone, h.reserveAs(*list.ShareSlug, *item.Id, "giver@example.com").StatusCode,
		"the anonymous reserve path")

	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "giver@example.com")
	assert.Equal(t, http.StatusGone, resp.StatusCode, "the co-buy contribute path")
}

// --- Q1: the giver is told ---------------------------------------------------

func TestArchivingTellsTheGiverHoldingAReservation(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	require.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *item.Id, "giver@example.com").StatusCode)

	before := len(h.email.Sent())
	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))

	sent := h.email.Sent()[before:]
	require.Len(t, sent, 1, "the giver is told, it does not go quiet (#419 Q1)")
	assert.Equal(t, "giver@example.com", sent[0].To)
	assert.Contains(t, sent[0].Body, "Item")
	assert.NotContains(t, sent[0].Body, "@", "the note names the item and no person")

	// A repeated archive is a no-op, not a second round of mail to every reserver.
	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))
	assert.Len(t, h.email.Sent()[before:], 1, "re-archiving must not re-notify")
}

func TestUnarchivingDoesNotEmailTheGiver(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	require.Equal(t, http.StatusCreated, h.reserveAs(*list.ShareSlug, *item.Id, "giver@example.com").StatusCode)
	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))

	before := len(h.email.Sent())
	require.Equal(t, http.StatusOK, h.unarchive(*item.Id).StatusCode)
	assert.Empty(t, h.email.Sent()[before:],
		"nothing about the giver's reservation changed — it was held, not released — so there is nothing to tell them")
}

// --- Q2: warn, do not block --------------------------------------------------

func TestArchivingWarnsAboutAnInFlightCoBuyButDoesNotBlock(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	// Two pledges that together cover the price propose a match: the handshake is
	// now in flight and neither giver has confirmed.
	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "one@example.com")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "two@example.com")
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	archiveResp, result := h.archive(*item.Id)
	require.Equal(t, http.StatusOK, archiveResp.StatusCode,
		"the list belongs to its owner: a co-buy in flight warns, it does not block (#419 Q2)")
	require.NotNil(t, result.Warnings)
	require.Len(t, *result.Warnings, 1)
	assert.Equal(t, gen.CobuyMatchPending, *(*result.Warnings)[0].Code)
	require.NotNil(t, result.Item)
	assert.NotNil(t, result.Item.ArchivedAt, "warned, and archived anyway")
}

func TestArchivingAQuietItemWarnsAboutNothing(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	// One pledge, no partner, so no match has been proposed and nothing is in
	// flight. This is the negative control for the warning: without it, a warning
	// list that is always populated would pass the test above just as well.
	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "one@example.com")
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	_, result := h.archive(*item.Id)
	require.NotNil(t, result.Warnings)
	assert.Empty(t, *result.Warnings)
}

// --- authorisation -----------------------------------------------------------

func TestArchiveRequiresOwningTheList(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)

	other, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(),
		storage.User{Name: "Bob", Role: storage.RoleOwner})
	require.NoError(t, err)
	otherToken := h.tokenFor(other.ID, h.tenant.ID)

	resp, _ := h.req(http.MethodPost, "/api/v1/items/"+*item.Id+"/archive", h.ownerHost(), otherToken, nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	resp, _ = h.req(http.MethodDelete, "/api/v1/items/"+*item.Id+"/archive", h.ownerHost(), otherToken, nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	resp, _ = h.req(http.MethodPost, "/api/v1/items/no-such-item/archive", h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- the other owner-side reads ----------------------------------------------

func TestArchivedItemsAreLeftOutOfTheExport(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	kept := h.createItem(*list.Id, "Still wanted", 1)
	done := h.createItem(*list.Id, "Finished with", 1)
	require.Equal(t, http.StatusOK, mustStatus(h.archive(*done.Id)))

	resp, body := h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/export?format=json",
		h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The export is deliberately state-free, so it has nowhere to record "this one
	// is finished". Carrying archived items anyway would make a re-import resurrect
	// them as live and reservable — the bug, through a different door.
	assert.Contains(t, string(body), "Still wanted")
	assert.NotContains(t, string(body), "Finished with")
	_ = kept
}

func TestArchivedItemsAreLeftOutOfTheListPreviews(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	require.Equal(t, http.StatusOK, mustStatus(h.archive(*item.Id)))

	resp, body := h.req(http.MethodGet, "/api/v1/lists", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	for _, l := range *decode[gen.ListPage](t, body).Items {
		if *l.Id != *list.Id {
			continue
		}
		if l.ItemPreviews != nil {
			for _, p := range *l.ItemPreviews {
				assert.NotEqual(t, *item.Id, *p.Id, "an archived item is not part of the list's preview cluster")
			}
		}
	}
}

// mustStatus unwraps the (response, decoded) pair the archive helper returns when
// only the status matters at the call site.
func mustStatus(resp *http.Response, _ gen.ItemArchiveResult) int { return resp.StatusCode }
