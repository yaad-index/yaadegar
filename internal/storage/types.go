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

// User is an owner within a tenant. Email is used server-side (e.g. decay notices)
// and may be empty. Username/PasswordHash back the password login method (ADR-0005):
// Username is the per-tenant login handle (nil = none), PasswordHash is the
// argon2id PHC-encoded hash (empty = no password credential, e.g. an
// OAuth/magic-link-only account). The plaintext password is never stored.
type User struct {
	ID           string
	TenantID     string
	Name         string
	Email        string
	Username     *string
	PasswordHash string
	CreatedAt    time.Time
}

// Admin is the instance-level superadmin (ADR-0005 §6): not tied to any tenant,
// authenticated on the separate /admin surface. PasswordHash is an argon2id hash;
// the plaintext is never stored.
type Admin struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
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
	// ReserverTier is the per-list reserver-identity tier override (ADR-0007): nil
	// inherits the instance default. Stored as a nullable column (NULL = inherit).
	ReserverTier *ReserverTier
	// ReserverConfirmWindowMinutes is the per-list email_confirmed confirm-window
	// override in minutes (ADR-0007): nil inherits the instance default. It mirrors
	// DecayDays' encoding — the nil/-1 mapping lives entirely in the storage path.
	ReserverConfirmWindowMinutes *int
	Active                       bool
	CreatedAt                    time.Time
	// ItemCount is a derived read field: the number of items on the list. It is
	// populated by reads (Get/GetBySlug/List) and left zero by Create.
	ItemCount int
}

// ReserverTier is the identity level a giver must meet to reserve on a list
// (ADR-0007). full_guest is the lowest-friction default; email_confirmed requires a
// verified email before the reservation activates; registered (deferred) requires a
// giver account.
type ReserverTier string

const (
	TierFullGuest      ReserverTier = "full_guest"
	TierEmailConfirmed ReserverTier = "email_confirmed"
	TierRegistered     ReserverTier = "registered"
)

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

// ReservationState tracks a reservation through its lifecycle. The email_confirmed
// tier adds an initial pending_confirmation gate; then the stale-reservation decay
// flow runs: (pending_confirmation →) active → reserver_notified → expired, with a
// "keep" click returning it to active. full_guest reservations are created active
// and skip pending_confirmation (ADR-0007 §5).
type ReservationState string

const (
	// StatePendingConfirmation is an email_confirmed reservation awaiting the giver's
	// email confirmation; it holds the item provisionally and either advances to
	// active (on confirm) or expires (confirm-window elapses). Never reached by
	// full_guest reservations.
	StatePendingConfirmation ReservationState = "pending_confirmation"
	StateActive              ReservationState = "active"
	StateReserverNotified    ReservationState = "reserver_notified"
	StateExpired             ReservationState = "expired"
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
	// State is the reservation lifecycle state; `active` (full_guest) or
	// `pending_confirmation` (email_confirmed) at creation.
	State ReservationState
	// StateAt stamps when State was last set; it drives the confirm, grace, and
	// expire windows. Defaults to CreatedAt.
	StateAt time.Time
	// ConfirmTokenHash is the hash of the one-time email-confirmation token for an
	// email_confirmed reservation (empty for full_guest). The raw token is emailed
	// once and never stored (like the capability token).
	ConfirmTokenHash string
	// EmailConfirmedAt is set when a pending_confirmation reservation is confirmed —
	// the un-backfillable "this email is verified" signal (system-only; drives
	// deliverable decay reminders and a future cross-list view keyed on verified
	// emails, #20). nil = never confirmed (full_guest, or still pending).
	EmailConfirmedAt *time.Time
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
	State          ReservationState
	LastActivityAt time.Time
	StateAt        time.Time
	// DecayDays is the list's period override (nil = inherit the instance
	// default). The sweeper resolves the effective period; it never compares this
	// raw value.
	DecayDays *int
	// ReserverConfirmWindowMinutes is the list's confirm-window override in minutes
	// (nil = inherit the instance default). The sweeper resolves the effective
	// window for a pending_confirmation reservation; it never compares this raw value.
	ReserverConfirmWindowMinutes *int
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
	// MatchActionTokenHash is the hash of a purpose-scoped, expiring token minted
	// when this contribution enters a proposed match and emailed to the giver, so a
	// cross-device confirm/decline link works without the capability cookie. It can
	// ONLY confirm/decline this contribution's match (never withdraw); it is cleared
	// when the match dissolves (#96, ADR-0002 §6).
	MatchActionTokenHash      string
	MatchActionTokenExpiresAt *time.Time
	CreatedAt                 time.Time
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
