// Package storage defines Yaadegar's persistence contract: the domain entities
// and the repository interfaces over them. Concrete drivers live in
// subpackages (see internal/storage/sqlstore). All data access is tenant-scoped
// by construction — see ADR-0003 and the Store documentation.
package storage

import "time"

// Visibility controls who can find a list. A list is always reachable by its
// opaque share_slug; visibility governs discovery, not the share link.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityUnlisted Visibility = "unlisted"
	VisibilityPrivate  Visibility = "private"
)

// Availability is an item's giver-facing state. It never encodes *who* reserved
// or is buying — only the state (ADR-0002 §5). It is derived from reservations,
// contributions, and matches; storage persists the underlying rows and #5
// computes this for API responses.
type Availability string

const (
	AvailabilityAvailable Availability = "available"
	AvailabilityReserved  Availability = "reserved"
	AvailabilityCoBuying  Availability = "co_buying"
	AvailabilityPurchased Availability = "purchased"
)

// ContributionStatus tracks a single pledge through the co-buying handshake.
type ContributionStatus string

const (
	ContributionPending   ContributionStatus = "pending"
	ContributionMatched   ContributionStatus = "matched"
	ContributionConfirmed ContributionStatus = "confirmed"
	ContributionDeclined  ContributionStatus = "declined"
	ContributionWithdrawn ContributionStatus = "withdrawn"
)

// MatchState tracks a co-buying handshake between contributions on one item.
type MatchState string

const (
	MatchProposed      MatchState = "proposed"
	MatchBothConfirmed MatchState = "both_confirmed"
	MatchDone          MatchState = "done"
	MatchDeclined      MatchState = "declined"
)

// TLSStatus is a custom domain's certificate state. Provisioning is deferred
// (ADR-0001); the column exists so the model is ready for it.
type TLSStatus string

const (
	TLSNone    TLSStatus = "none"
	TLSPending TLSStatus = "pending"
	TLSActive  TLSStatus = "active"
)

// Money is an exact monetary amount: an integer count of minor units (e.g.
// cents) plus an ISO-4217 currency. Never a float (ADR-0002 §10). The zero
// value means "no amount".
type Money struct {
	AmountMinor int64
	Currency    string
}

// Tenant is one addressable space (a subdomain and/or custom domains). Every
// other entity belongs to exactly one tenant.
type Tenant struct {
	ID        string
	Subdomain string
	CreatedAt time.Time
}

// User is an owner within a tenant. The authentication mechanism is deferred to
// a later ADR (ADR-0002 §4); this is the persisted identity only. Email is used
// server-side (e.g. decay notices) and may be empty until real auth lands.
type User struct {
	ID        string
	TenantID  string
	Name      string
	Email     string
	CreatedAt time.Time
}

// List is an owner's wishlist. ShareSlug is the opaque, unguessable handle used
// by the public giver surface.
type List struct {
	ID         string
	TenantID   string
	OwnerID    string
	Title      string
	Visibility Visibility
	ShareSlug  string
	EventDate  *time.Time // date-only; nil = none. After it, the list auto-disables.
	// DecayDays is the reservation-decay period override: nil inherits the
	// instance default, 0 means off, N means N days. The nil/-1 encoding is
	// handled entirely in the storage scan/insert path.
	DecayDays *int
	Active    bool
	CreatedAt time.Time
	// ItemCount is a derived read field: the number of items on the list. It is
	// populated by reads (Get/GetBySlug/List) and left zero by Create.
	ItemCount int
}

// Item is one entry on a list. Nil pointers are absent optional fields.
type Item struct {
	ID             string
	TenantID       string
	ListID         string
	Name           string
	URL            *string
	ImageURL       *string
	Price          *Money
	Note           *string
	Priority       int
	QuantityWanted int
	CreatedAt      time.Time
}

// ReservationDecayState tracks a reservation through the stale-reservation decay
// flow: active → reserver_notified → expired, with a "keep" click returning it to
// active. Only `active` is set at creation.
type ReservationDecayState string

const (
	DecayActive           ReservationDecayState = "active"
	DecayReserverNotified ReservationDecayState = "reserver_notified"
	DecayExpired          ReservationDecayState = "expired"
)

// Reservation is an anonymous giver's claim on an item. GiverName/GiverEmail are
// server-side only (decay reminders) and never shown to others (ADR-0002 §5).
// TokenHash is a hash of the one-time capability token — never the raw token
// (ADR-0003 §3).
type Reservation struct {
	ID         string
	TenantID   string
	ItemID     string
	GiverName  *string
	GiverEmail *string
	Quantity   int
	TokenHash  string
	CreatedAt  time.Time
	// LastActivityAt seeds the decay clock; set to CreatedAt on creation.
	LastActivityAt time.Time
	// IsGroup marks a reservation opened as part of group co-buying (#7).
	IsGroup bool
	// DecayState is the stale-reservation lifecycle state; `active` at creation.
	DecayState ReservationDecayState
	// DecayStateAt stamps when DecayState was last set; it drives the grace and
	// expire windows. Defaults to CreatedAt.
	DecayStateAt time.Time
	// DecayReleaseTokenHash / DecayKeepTokenHash are the hashes of the one-click
	// release and keep tokens minted at reserver_notified (empty = none). The raw
	// tokens are emailed once and never stored (like the capability token).
	DecayReleaseTokenHash string
	DecayKeepTokenHash    string
}

// DecayCandidate is a reservation that the decay sweep may need to advance, with
// the list's decay period joined in. Returned by the (system-level, cross-tenant)
// Store.DecayCandidates read; the actual transition is applied tenant-scoped.
type DecayCandidate struct {
	TenantID       string
	ReservationID  string
	ItemID         string
	ItemName       string
	GiverEmail     *string // reserver's email (optional)
	DecayState     ReservationDecayState
	LastActivityAt time.Time
	DecayStateAt   time.Time
	// DecayDays is the list's period override (nil = inherit the instance
	// default). The sweeper resolves the effective period; it never compares this
	// raw value.
	DecayDays *int
}

// Contribution is a giver's pledge toward co-buying an item. ContactEmail is
// revealed to a co-buyer only after a mutually-confirmed match (ADR-0002 §6).
// TokenHash follows the same rule as Reservation.
type Contribution struct {
	ID           string
	TenantID     string
	ItemID       string
	Pledged      Money
	GiverName    *string
	ContactEmail string
	Status       ContributionStatus
	MatchID      *string
	TokenHash    string
	CreatedAt    time.Time
}

// Match is the two-sided co-buying handshake linking contributions on one item.
type Match struct {
	ID              string
	TenantID        string
	ItemID          string
	State           MatchState
	ContributionIDs []string
	CreatedAt       time.Time
}

// Domain is a custom hostname pointed at a tenant (bring-your-own-domain).
type Domain struct {
	ID          string
	TenantID    string
	Hostname    string
	CNAMETarget string
	Verified    bool
	TLSStatus   TLSStatus
	// VerificationToken is a stable DNS TXT proof-of-control challenge (ADR-0004
	// §2). It is not a secret capability: stored plaintext, exposed to the owner.
	VerificationToken string
	CreatedAt         time.Time
}
