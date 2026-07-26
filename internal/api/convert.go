package api

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// ptr returns a pointer to v — the generated optional fields are all pointers.
func ptr[T any](v T) *T { return &v }

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
	return gen.List{
		Id:         ptr(l.ID),
		Title:      ptr(l.Title),
		Visibility: ptr(gen.ListVisibility(l.Visibility)),
		ShareSlug:  ptr(l.ShareSlug),
		EventDate:  toGenDate(l.EventDate),
		DecayDays:  ptr(l.DecayDays),
		Active:     ptr(l.Active),
		ItemCount:  ptr(l.ItemCount),
		CreatedAt:  ptr(l.CreatedAt),
	}
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
	}
}

// toGenPublicItem maps a stored item to the giver-facing PublicItem: availability
// state and funded amount only, never who reserved or is buying (ADR-0002 §5/§6).
func toGenPublicItem(it storage.Item, avail storage.Availability, funded storage.Money) gen.PublicItem {
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
	}
}

func toGenUser(u storage.User, t storage.Tenant) gen.User {
	return gen.User{
		Id:   ptr(u.ID),
		Name: ptr(u.Name),
		Tenant: &struct {
			Id        *string `json:"id,omitempty"`
			Subdomain *string `json:"subdomain,omitempty"`
		}{
			Id:        ptr(t.ID),
			Subdomain: ptr(t.Subdomain),
		},
	}
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
