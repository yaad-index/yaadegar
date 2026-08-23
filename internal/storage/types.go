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
	// ContributionExpired is a terminal status set by the match auto-expiry sweep
	// (#101) when the confirm window elapses on a still-proposed match: every pledge
	// in that match is driven here, distinct from the giver-initiated declined /
	// withdrawn so the outcome reads honestly ("this group buy expired", not
	// "someone declined"). Terminal like the other two, so it frees the item.
	ContributionExpired ContributionStatus = "expired"
)

// MatchState tracks a co-buying handshake between contributions on one item.
type MatchState string

const (
	MatchProposed      MatchState = "proposed"
	MatchBothConfirmed MatchState = "both_confirmed"
	MatchDone          MatchState = "done"
	MatchDeclined      MatchState = "declined"
	// MatchExpired is the terminal state set by the match auto-expiry sweep (#101)
	// when a match sits proposed past the confirm window: the match and every pledge
	// on it are torn down so the item frees for either track. Distinct from declined
	// (a giver's action) so the state reads truthfully.
	MatchExpired MatchState = "expired"
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
	// OAuthGoogleEnabled is the per-tenant Google-login toggle (ADR-0008 §6),
	// default off. It only takes effect when the instance also has a Google client
	// configured (env, fail-closed); on an instance with no client it is inert.
	OAuthGoogleEnabled bool
}

// UserRole is a user's per-tenant role (ADR-0009): an owner authors and owns
// lists; a giver is a first-class reserver account (which is what makes the
// `registered` reserver tier real). The instance-admin capability is orthogonal to
// this axis (ADR-0009 §3) and is not a value here.
type UserRole string

const (
	RoleOwner UserRole = "owner"
	RoleGiver UserRole = "giver"
)

// RegistrationPolicy is the instance-wide self-registration policy (ADR-0009
// Decision 2, ADR-0012). It gates the unauthenticated self-registration endpoint
// server-side. `disabled` is the fail-closed default — an existing instance keeps
// its unchanged behavior (no self-registration) until the operator opts in.
type RegistrationPolicy string

const (
	// RegistrationDisabled turns self-registration off (the default): the register
	// endpoint answers 403 on every instance that has not opted in.
	RegistrationDisabled RegistrationPolicy = "disabled"
	// RegistrationGiversOnly lets anyone self-register a giver account (a first-class
	// reserver, ADR-0009); they cannot author lists.
	RegistrationGiversOnly RegistrationPolicy = "givers_only"
	// RegistrationOwnersAllowed lets anyone self-register a full owner account (can
	// author lists) — the most open policy.
	RegistrationOwnersAllowed RegistrationPolicy = "owners_allowed"
)

// User account lifecycle states (ADR-0012). Existing accounts and operator-created
// ones are `active`; a self-registered account is `pending` until it verifies its
// email, at which point it flips to `active`. A pending account cannot hold a
// session (the login gate rejects it, like a ban).
const (
	UserStatusActive  = "active"
	UserStatusPending = "pending"
)

// User is an account within a tenant. Email is used server-side (e.g. decay
// notices) and may be empty. Username/PasswordHash back the password login method
// (ADR-0005): Username is the per-tenant login handle (nil = none), PasswordHash is
// the argon2id PHC-encoded hash (empty = no password credential, e.g. an
// OAuth/magic-link-only account). The plaintext password is never stored.
type User struct {
	ID           string
	TenantID     string
	Name         string
	Email        string
	Username     *string
	PasswordHash string
	// Role is the per-tenant role (ADR-0009); defaults to owner for every account
	// that existed before the roles model (the migration default).
	Role UserRole
	// Banned marks an instance-admin ban: a banned account cannot log in or hold a
	// session (enforced at issue and at the owner middleware's per-request load).
	Banned bool
	// IsAdmin is the instance-admin capability (ADR-0010): an owner account that
	// also carries this flag reaches the non-tenant-scoped /admin surface. It is
	// orthogonal to Role — the capability, not a separate identity, is what makes an
	// account an admin. requireAdmin reads it per-request from the token's home
	// tenant, so revoking it takes effect immediately.
	IsAdmin bool
	// CredentialVersion is the session-invalidation counter (ADR-0011): every
	// password mutation increments it, and an issued session JWT pins the value it
	// was minted at. The owner middleware rejects a token whose `cver` claim no
	// longer matches this stored value, so a password change/reset/set-password
	// revokes all prior sessions immediately. Starts at 1 (the migration default);
	// Create seeds a new account at 1.
	CredentialVersion int
	// Status is the account lifecycle state (ADR-0012): `active` for existing and
	// operator-created accounts (the migration default), `pending` for a
	// self-registered account awaiting email verification. A pending account cannot
	// log in (the login gate rejects it like a ban); verify flips it to active. Empty
	// is treated as active at Create.
	Status    string
	CreatedAt time.Time
}

// PasswordResetToken is a single-use, short-TTL credential for the forgot-password
// flow (ADR-0011 cut 3). TokenHash is the sha256 of the raw token; the raw value is
// emailed once and never stored (like the reservation capability token, ADR-0003 §3).
// UsedAt is nil until the token is claimed at confirm — the single-use guard.
type PasswordResetToken struct {
	ID        string
	TenantID  string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// EmailVerificationToken is a single-use, short-TTL credential for the email
// self-registration flow (ADR-0012). TokenHash is the sha256 of the raw token; the
// raw value is emailed once and never stored (like the password-reset token,
// ADR-0003 §3). UsedAt is nil until the token is claimed at verify — the single-use
// guard. Verifying it flips the pending account to active.
type EmailVerificationToken struct {
	ID        string
	TenantID  string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
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
	// ReserverTier is the per-list reserver-identity tier override (ADR-0007): nil
	// inherits the instance default. Stored as a nullable column (NULL = inherit).
	ReserverTier *ReserverTier
	// ReserverConfirmWindowMinutes is the per-list email_confirmed confirm-window
	// override in minutes (ADR-0007): nil inherits the instance default. It mirrors
	// DecayDays' encoding — the nil/-1 mapping lives entirely in the storage path.
	ReserverConfirmWindowMinutes *int
	// AllowCobuy is the list-level default for whether items may be co-bought (#100).
	// It defaults to true (co-buying enabled); an item inherits it unless the item
	// sets its own override. A new list is always created enabled — the owner opts
	// out later via update.
	AllowCobuy bool
	// ThankYouTemplate is the list-level default owner→giver thank-you note (#22):
	// a plain-text body emailed to a reserver's contact when their reservation goes
	// active. "" = no note (feature off). Items inherit it unless they override it.
	// A `{item}` token in the body is replaced with the item name; it carries no
	// giver identity (the owner stays blind to who reserved).
	ThankYouTemplate string
	// Description is the owner-editable list description (#143): a light-markdown
	// body shown to givers at the top of the public list (and to the owner). "" =
	// no description. Rendered to sanitized HTML in the frontend load via the same
	// renderNote path item notes use (ADR-0006); stored raw here.
	Description string
	Active      bool
	CreatedAt   time.Time
	// ItemCount is a derived read field: the number of items on the list. It is
	// populated by reads (Get/GetBySlug/List) and left zero by Create.
	ItemCount int
	// ItemPreviews holds up to the first few items' thumbnails for the list-summary
	// preview cluster (#207), in item display order (priority DESC, created_at, id).
	// It is populated only by the List (index) read — the dashboard summary — and is
	// left nil by Get/GetBySlug/Create, which have no preview cluster to feed.
	ItemPreviews []ItemPreview
}

// ItemPreview is one item's thumbnail for the list-summary preview cluster (#207):
// its id and its own image URL (nil = the item has no image, so the card renders an
// accent-tinted placeholder glyph). It deliberately carries no reserver or
// availability data — the cluster shows the list's objects, never who reserved them.
type ItemPreview struct {
	ID       string
	ImageURL *string
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
	// AllowCobuy overrides the list's co-buy default for this item (#100): nil
	// inherits the list, non-nil forces on/off. Resolved against List.AllowCobuy.
	AllowCobuy *bool
	// ThankYouTemplate overrides the list's thank-you note for this item (#22): nil
	// inherits the list default, "" is an explicit per-item opt-out (no note), any
	// other value overrides the body. Resolved against List.ThankYouTemplate.
	ThankYouTemplate *string
	CreatedAt        time.Time
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
	// ReserverUserID binds a reservation to the authenticated account that made it
	// (ADR-0012 Decision 4 / cut 3), for the reserver's own "things I've reserved"
	// dashboard (#20). nil for an anonymous (full_guest / email_confirmed) reserve.
	// System-only: it gates the `registered` tier and powers the reserver's own view,
	// and never surfaces on any owner-facing response (ADR-0002 §5).
	ReserverUserID *string
}

// ReserverReservation is a read-model row for the reserver's "things I've reserved"
// dashboard (ADR-0012 Decision 4 / #20): a reservation the account made, joined to
// its item and list for display. It is only ever returned on the reserver's own
// surface, keyed on reserver_user_id, and carries no other reserver's identity.
type ReserverReservation struct {
	ReservationID string
	ItemID        string
	ItemName      string
	ListTitle     string
	ShareSlug     string
	Quantity      int
	State         ReservationState
	CreatedAt     time.Time
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

// MatchExpiryCandidate is a still-proposed match the auto-expiry sweep may need to
// dissolve, carrying just enough to apply the transition tenant-scoped: its tenant,
// id, item (for the row lock), and proposal time (the window is measured from it).
// Returned by the system-level, cross-tenant Store.ExpiredMatchCandidates read.
type MatchExpiryCandidate struct {
	TenantID  string
	MatchID   string
	ItemID    string
	CreatedAt time.Time
}

// OAuthProvider identifies an external identity provider. Only Google ships in v1
// (ADR-0008); the column and this type leave room for others without a schema
// change.
type OAuthProvider string

const (
	OAuthProviderGoogle OAuthProvider = "google"
)

// OAuthIdentity links a verified external identity to an owner within one tenant
// (ADR-0008 §5–6). Subject is the provider's stable, opaque user id (Google `sub`),
// the durable link key; Email is the address at link time (informational — the
// link never re-resolves by email). One owner may hold at most one identity per
// provider per tenant; (tenant_id, provider, subject) is unique, tenant-scoped.
type OAuthIdentity struct {
	ID        string
	TenantID  string
	UserID    string
	Provider  OAuthProvider
	Subject   string
	Email     string
	CreatedAt time.Time
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
