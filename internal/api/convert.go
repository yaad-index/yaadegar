package api

import (
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/settings"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// ptr returns a pointer to v — the generated optional fields are all pointers.
func ptr[T any](v T) *T { return &v }

// resolveAllowCobuy resolves whether an item may be co-bought (#100): the item's
// own override if set, otherwise the list-level default. This is the single choke
// point for the policy — the giver surface and the contribute gate both use it.
func resolveAllowCobuy(item storage.Item, list storage.List) bool {
	return settings.Resolve(item.AllowCobuy, list.AllowCobuy)
}

// resolveThankYou resolves the owner→giver thank-you note for an item (#22): the
// item's own override if set (an explicit "" is a per-item opt-out), else the list
// default. An empty result means no note is sent.
func resolveThankYou(item storage.Item, list storage.List) string {
	return settings.Resolve(item.ThankYouTemplate, list.ThankYouTemplate)
}

// renderThankYou substitutes the single supported {item} token with the item name.
// It is the only token — the note never carries giver identity (#22 anonymity).
func renderThankYou(tmpl, itemName string) string {
	return strings.ReplaceAll(tmpl, "{item}", itemName)
}

func toGenDate(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}

func fromGenDate(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

func toGenMoney(m *storage.Money) *gen.Money {
	if m == nil {
		return nil
	}
	return &gen.Money{AmountMinor: int(m.AmountMinor), Currency: m.Currency}
}

func fromGenMoney(m *gen.Money) *storage.Money {
	if m == nil {
		return nil
	}
	return &storage.Money{AmountMinor: int64(m.AmountMinor), Currency: m.Currency}
}

func fromGenVisibility(v *gen.ListVisibility) storage.Visibility {
	if v == nil {
		return ""
	}
	return storage.Visibility(*v)
}

func toGenList(l storage.List) gen.List {
	var tier *string
	if l.ReserverTier != nil {
		t := string(*l.ReserverTier)
		tier = &t
	}
	return gen.List{
		Id:                    ptr(l.ID),
		Title:                 ptr(l.Title),
		Visibility:            ptr(gen.ListVisibility(l.Visibility)),
		ShareSlug:             ptr(l.ShareSlug),
		EventDate:             toGenDate(l.EventDate),
		DecayDays:             l.DecayDays,                    // *int: nil = inheriting the instance default
		ReserverTier:          tier,                           // nil = inheriting the instance default
		ReserverConfirmWindow: l.ReserverConfirmWindowMinutes, // *int minutes: nil = inherit
		AllowCobuy:            ptr(l.AllowCobuy),              // the list-level co-buy default (#100)
		ThankYouTemplate:      ptr(l.ThankYouTemplate),        // list-level thank-you default (#22)
		Description:           ptr(l.Description),             // owner-editable list description (#143)
		Active:                ptr(l.Active),
		ItemCount:             ptr(l.ItemCount),
		ItemPreviews:          toGenItemPreviews(l.ItemPreviews), // nil except on the list-index summary (#207)
		CreatedAt:             ptr(l.CreatedAt),
	}
}

// toGenItemPreviews maps the list-summary preview cluster (#207). It returns nil
// (the field is omitted) when there are no previews — an empty list, or any read
// other than the list index — so the single-list responses stay unchanged.
func toGenItemPreviews(ps []storage.ItemPreview) *[]gen.ItemPreview {
	if len(ps) == 0 {
		return nil
	}
	out := make([]gen.ItemPreview, 0, len(ps))
	for _, p := range ps {
		out = append(out, gen.ItemPreview{Id: ptr(p.ID), ImageUrl: p.ImageURL})
	}
	return &out
}

// parseReserverTier validates and maps the request's reserver_tier override. nil
// (absent/null) inherits the instance default; a present value must be a known
// tier or it is a 400. registered is accepted (it is a valid tier) but its
// enforcement is deferred until a giver-account system exists (ADR-0007 §6).
func parseReserverTier(s *string) (*storage.ReserverTier, bool) {
	if s == nil {
		return nil, true
	}
	switch storage.ReserverTier(*s) {
	case storage.TierFullGuest, storage.TierEmailConfirmed, storage.TierRegistered:
		t := storage.ReserverTier(*s)
		return &t, true
	}
	return nil, false
}

// toGenItem maps a stored item plus its derived availability and reserved count
// to the owner Item. Both are aggregates — a state and a count — and carry no
// reserver identity (ADR-0002 §5).
func toGenItem(it storage.Item, avail storage.Availability, reservedQty int) gen.Item {
	return gen.Item{
		Id:               ptr(it.ID),
		ListId:           ptr(it.ListID),
		Name:             ptr(it.Name),
		Url:              it.URL,
		ImageUrl:         it.ImageURL,
		Price:            toGenMoney(it.Price),
		Note:             it.Note,
		Priority:         ptr(it.Priority),
		QuantityWanted:   ptr(it.QuantityWanted),
		Availability:     ptr(gen.ItemAvailability(avail)),
		ReservedQuantity: ptr(reservedQty),
		AllowCobuy:       it.AllowCobuy,       // *bool: nil = inheriting the list default (#100)
		ThankYouTemplate: it.ThankYouTemplate, // *string: nil = inheriting the list default (#22)
	}
}

// toGenPublicItem maps a stored item to the giver-facing PublicItem: availability
// state and funded amount only, never who reserved or is buying (ADR-0002 §5/§6).
func toGenPublicItem(it storage.Item, avail storage.Availability, funded storage.Money, allowCobuy bool) gen.PublicItem {
	var amountFunded *gen.Money
	if funded.AmountMinor > 0 {
		amountFunded = toGenMoney(&funded)
	}
	return gen.PublicItem{
		Id:           ptr(it.ID),
		Name:         ptr(it.Name),
		Url:          it.URL,
		ImageUrl:     it.ImageURL,
		Price:        toGenMoney(it.Price),
		Note:         it.Note,
		Availability: ptr(gen.ItemAvailability(avail)),
		AmountFunded: amountFunded,
		// The resolved co-buy eligibility (#100). The giver surface shows the chip-in
		// affordance only when this is true AND the item is priced.
		AllowCobuy: ptr(allowCobuy),
	}
}

func toGenUser(u storage.User, t storage.Tenant) gen.User {
	return gen.User{
		Id:      ptr(u.ID),
		Name:    ptr(u.Name),
		Role:    ptr(gen.UserRole(u.Role)),
		IsAdmin: ptr(u.IsAdmin),
		Tenant: &struct {
			Id        *string `json:"id,omitempty"`
			Subdomain *string `json:"subdomain,omitempty"`
		}{
			Id:        ptr(t.ID),
			Subdomain: ptr(t.Subdomain),
		},
	}
}

// toGenContribution maps a stored contribution to its giver-facing view. It
// carries no other party's contact — only this contribution's own fields.
func toGenContribution(c storage.Contribution) gen.Contribution {
	return gen.Contribution{
		Id:      ptr(c.ID),
		ItemId:  ptr(c.ItemID),
		Pledged: toGenMoney(&c.Pledged),
		Status:  ptr(gen.ContributionStatus(c.Status)),
		MatchId: c.MatchID,
	}
}

// toGenMatch maps a match. Contacts are populated ONLY when the match is
// both_confirmed — this is the serialization-boundary enforcement of the
// reveal-only-after-mutual-confirmation rule (ADR-0002 §6). Before that, no
// contact is emitted regardless of what the caller passes.
func toGenMatch(m storage.Match, contacts []string) gen.Match {
	ids := m.ContributionIDs
	g := gen.Match{
		Id:              ptr(m.ID),
		ItemId:          ptr(m.ItemID),
		State:           ptr(gen.MatchState(m.State)),
		ContributionIds: &ids,
	}
	if m.State == storage.MatchBothConfirmed {
		emails := make([]openapi_types.Email, 0, len(contacts))
		for _, e := range contacts {
			emails = append(emails, openapi_types.Email(e))
		}
		g.Contacts = &emails
	}
	return g
}

// deriveAvailability turns storage aggregates into the public availability state.
// A funded pledge means co-buying; a fully-reserved item (claimed units meet or
// exceed the wanted quantity) means reserved; otherwise units remain, so it is
// available. `purchased` is not derivable until later issues add that signal.
func deriveAvailability(quantityWanted, reservedQty int, funded storage.Money) storage.Availability {
	switch {
	case funded.AmountMinor > 0:
		return storage.AvailabilityCoBuying
	case quantityWanted > 0 && reservedQty >= quantityWanted:
		return storage.AvailabilityReserved
	default:
		return storage.AvailabilityAvailable
	}
}
