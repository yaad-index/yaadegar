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
// a later ADR (ADR-0002 §4); this is the persisted identity only.
type User struct {
	ID        string
	TenantID  string
	Name      string
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
	DecayDays  int        // 0 = decay off
	Active     bool
	CreatedAt  time.Time
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
	CreatedAt   time.Time
}
