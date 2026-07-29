package storage

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors drivers must return so callers can branch without depending on
// a specific database's error types.
var (
	// ErrNotFound is returned when a row does not exist within the caller's
	// tenant scope. Note: a row that exists in another tenant is reported as
	// ErrNotFound, never revealed (ADR-0003 §2).
	ErrNotFound = errors.New("storage: not found")
	// ErrConflict is returned on a uniqueness or state conflict (e.g. a
	// duplicate subdomain, share slug, or custom domain).
	ErrConflict = errors.New("storage: conflict")
	// ErrCapacityExceeded is returned by the capacity-guarded creates when an
	// insert would push a total past its cap — a reservation beyond quantity_wanted
	// or a contribution beyond the item price. The check and insert are atomic.
	ErrCapacityExceeded = errors.New("storage: capacity exceeded")
	// ErrInvalidSubdomain is returned by CreateTenant for an empty subdomain. The
	// full format + reserved-name policy is enforced at the provisioning boundary
	// (see internal/tenant); this is the storage floor.
	ErrInvalidSubdomain = errors.New("storage: invalid subdomain")
)

// Driver selects a backing database.
type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
)

// Config selects and locates the database. It is populated from the app config
// (file < env < flag) and passed to a driver's Open.
type Config struct {
	Driver Driver
	DSN    string
}

// Page is an offset/limit window (ADR-0002 §8). Repositories that return
// collections also return the unpaged total.
type Page struct {
	Limit  int
	Offset int
}

// Store is the top-level persistence handle. Tenant resolution and migrations
// live here; **all domain data access is reached only through ForTenant**, so no
// query can be issued without a tenant scope (ADR-0003 §2). CreateTenant and the
// TenantBy* lookups are the only unscoped operations — they are the doors to a
// tenant scope, not a way around it.
type Store interface {
	// Migrate applies all pending migrations for the driver.
	Migrate(ctx context.Context) error

	// CreateTenant provisions a new tenant. ErrConflict if the subdomain is taken.
	CreateTenant(ctx context.Context, t Tenant) (Tenant, error)
	// TenantByID looks up a tenant by id.
	TenantByID(ctx context.Context, id string) (Tenant, error)
	// TenantBySubdomain resolves the default per-user subdomain to its tenant.
	TenantBySubdomain(ctx context.Context, subdomain string) (Tenant, error)
	// TenantByCustomDomain resolves a verified bring-your-own custom hostname to
	// its tenant. Host-string parsing (which of the two lookups to use) is the
	// caller's concern; storage stays free of base-domain policy.
	TenantByCustomDomain(ctx context.Context, hostname string) (Tenant, error)

	// EnsureAdmin idempotently provisions the instance-level superadmin from
	// configuration: it inserts the admin, or updates the stored password hash when
	// the username already exists (so rotating the configured hash takes effect).
	// Instance-level — not tenant-scoped (ADR-0005 §6).
	EnsureAdmin(ctx context.Context, username, passwordHash string) (Admin, error)
	// AdminByUsername looks up the superadmin by username; ErrNotFound if absent.
	AdminByUsername(ctx context.Context, username string) (Admin, error)
	// AdminByID looks up the superadmin by id; ErrNotFound if absent.
	AdminByID(ctx context.Context, id string) (Admin, error)

	// ForTenant returns a data-access handle bound to exactly one tenant. Every
	// repository reached through it filters and stamps tenant_id from t.ID; there
	// is no unscoped repository.
	ForTenant(t Tenant) TenantStore

	// DecayCandidates returns non-expired reservations on active, decaying lists
	// (decay_days > 0) across all tenants — the input to a decay sweep. This is a
	// trusted system-level read (the sweeper), the same class of unscoped
	// operation as Migrate; the transition it drives is applied tenant-scoped via
	// ForTenant. Each candidate carries its tenant id and the list's decay period.
	DecayCandidates(ctx context.Context) ([]DecayCandidate, error)

	// Ping verifies connectivity.
	Ping(ctx context.Context) error
	// Close releases the underlying connection pool.
	Close() error
}

// TenantStore hands out repositories already bound to one tenant. Because the
// binding is captured when the handle is created (not passed per call), no
// repository method can reach another tenant's data.
type TenantStore interface {
	Users() UserRepo
	Lists() ListRepo
	Items() ItemRepo
	Reservations() ReservationRepo
	Contributions() ContributionRepo
	Matches() MatchRepo
	Domains() DomainRepo
}

// UserRepo persists owners within the bound tenant.
type UserRepo interface {
	Create(ctx context.Context, u User) (User, error)
	Get(ctx context.Context, id string) (User, error)
	// ByUsername looks a user up by their per-tenant login handle, for password
	// authentication. Returns ErrNotFound when no user has that username.
	ByUsername(ctx context.Context, username string) (User, error)
}

// ListRepo persists lists within the bound tenant. Ownership lives in a join table
// (list_owners); v1 enforces exactly one owner per list at Create, but the schema
// is multi-owner-capable (ADR-0005 §7).
type ListRepo interface {
	// Create persists a list owned by ownerID, atomically recording that ownership.
	// It enforces the v1 single-owner rule (the list is created with exactly one
	// owner). The List's own OwnerID field is ignored in favor of ownerID.
	Create(ctx context.Context, l List, ownerID string) (List, error)
	Get(ctx context.Context, id string) (List, error)
	// GetBySlug backs the public giver surface. It still resolves within the
	// bound tenant — the slug is unique per tenant.
	GetBySlug(ctx context.Context, shareSlug string) (List, error)
	// List returns the lists ownerID owns, resolved through the join table.
	List(ctx context.Context, ownerID string, p Page) ([]List, int, error)
	Update(ctx context.Context, l List) (List, error)
	Delete(ctx context.Context, id string) error

	// IsOwner reports whether userID is an owner of listID (the owner-surface
	// authorization check). False for a nonexistent or non-owned list.
	IsOwner(ctx context.Context, listID, userID string) (bool, error)
	// Owners lists the user ids that own listID.
	Owners(ctx context.Context, listID string) ([]string, error)
	// AddOwner records userID as an owner of listID. In v1 it enforces the
	// single-owner rule, returning ErrConflict if the list already has an owner
	// (co-ownership is deferred to #25). Idempotent for the existing sole owner.
	AddOwner(ctx context.Context, listID, userID string) error
	// RemoveOwner removes userID as an owner of listID (no-op if absent).
	RemoveOwner(ctx context.Context, listID, userID string) error
}

// ItemRepo persists items within the bound tenant.
type ItemRepo interface {
	Create(ctx context.Context, it Item) (Item, error)
	Get(ctx context.Context, id string) (Item, error)
	ListByList(ctx context.Context, listID string, p Page) ([]Item, int, error)
	Update(ctx context.Context, it Item) (Item, error)
	Delete(ctx context.Context, id string) error

	// ReservedQuantity sums active reservations on an item, so callers can derive
	// availability without an N+1 read.
	ReservedQuantity(ctx context.Context, itemID string) (int, error)
	// FundedAmount sums non-terminal contribution pledges on an item.
	FundedAmount(ctx context.Context, itemID string) (Money, error)

	// ReservedQuantitiesByList returns reserved quantity per item for every item
	// on a list, in one query — the batch form used to derive availability across
	// a whole list without N+1 reads. Items with no reservations are absent from
	// the map (treat as zero).
	ReservedQuantitiesByList(ctx context.Context, listID string) (map[string]int, error)
	// FundedAmountsByList returns funded amount per item for every item on a list,
	// in one query. Items with no contributions are absent from the map.
	FundedAmountsByList(ctx context.Context, listID string) (map[string]Money, error)
}

// ReservationRepo persists reservations within the bound tenant.
type ReservationRepo interface {
	Create(ctx context.Context, r Reservation) (Reservation, error)
	// CreateWithinCapacity atomically inserts a reservation only if the item's
	// total reserved quantity would not exceed wantedQty; otherwise it returns
	// ErrCapacityExceeded (and ErrNotFound if the item is gone). The check and
	// insert run in one transaction with the item row locked, so concurrent
	// reservers cannot oversell.
	CreateWithinCapacity(ctx context.Context, r Reservation, wantedQty int) (Reservation, error)
	Get(ctx context.Context, id string) (Reservation, error)
	// ByTokenHash looks a reservation up by the hash of its capability token.
	ByTokenHash(ctx context.Context, tokenHash string) (Reservation, error)
	// ByDecayReleaseTokenHash / ByDecayKeepTokenHash look a reservation up by the
	// hash of its one-click release / keep token.
	ByDecayReleaseTokenHash(ctx context.Context, tokenHash string) (Reservation, error)
	ByDecayKeepTokenHash(ctx context.Context, tokenHash string) (Reservation, error)
	ListByItem(ctx context.Context, itemID string) ([]Reservation, error)
	Delete(ctx context.Context, id string) error

	// The decay transitions below are each row-locked and idempotent: they act
	// only if the reservation is still in the expected source state, returning
	// false (no error) otherwise — so concurrent sweeps/keeps/releases are safe
	// no-ops, and a caller must gate any email on a true return.

	// MarkReserverNotified moves active → reserver_notified, stamping state_at
	// and the minted keep/release token hashes.
	MarkReserverNotified(ctx context.Context, reservationID string, at time.Time, keepTokenHash, releaseTokenHash string) (bool, error)
	// MarkExpired moves reserver_notified → expired, leaving the token hashes in
	// place (a late click resolves to 410, not 404).
	MarkExpired(ctx context.Context, reservationID string, at time.Time) (bool, error)
	// ExpirePending moves pending_confirmation → expired when an email_confirmed
	// reservation's confirm-window elapses unconfirmed (ADR-0007 §3). Row-locked
	// single-winner; a no-op if it was already confirmed (active) or expired.
	ExpirePending(ctx context.Context, reservationID string, at time.Time) (bool, error)
	// Renew moves reserver_notified → active on a "keep" click: it resets the decay
	// clock (last_activity_at, state_at) and clears both one-click tokens.
	Renew(ctx context.Context, reservationID string, at time.Time) (bool, error)
}

// ContributionRepo persists co-buying pledges within the bound tenant.
type ContributionRepo interface {
	Create(ctx context.Context, c Contribution) (Contribution, error)
	// CreateWithinCapacity atomically inserts a contribution only if the item's
	// total non-terminal pledged amount would not exceed priceMinor; otherwise
	// ErrCapacityExceeded (ErrNotFound if the item is gone). Shares the item lock
	// with reserve so contribute cannot overfund under concurrency.
	CreateWithinCapacity(ctx context.Context, c Contribution, priceMinor int64) (Contribution, error)
	Get(ctx context.Context, id string) (Contribution, error)
	ByTokenHash(ctx context.Context, tokenHash string) (Contribution, error)
	ListByItem(ctx context.Context, itemID string) ([]Contribution, error)
	// Update persists status and match linkage.
	Update(ctx context.Context, c Contribution) (Contribution, error)
	Delete(ctx context.Context, id string) error
}

// MatchRepo persists co-buying matches within the bound tenant.
type MatchRepo interface {
	Create(ctx context.Context, m Match) (Match, error)
	Get(ctx context.Context, id string) (Match, error)
	ListByItem(ctx context.Context, itemID string) ([]Match, error)
	// Update persists the match state transition.
	Update(ctx context.Context, m Match) (Match, error)
	// ConfirmContribution atomically marks one contribution confirmed within the
	// match and, if that leaves every contribution on the match confirmed while the
	// match is still proposed, transitions the match to both_confirmed — all under
	// the item row lock (the same lock reserve/contribute use), so concurrent
	// confirms serialize and the completion happens exactly once. completedNow is
	// true only for the call that performs the transition; contribs holds the
	// match's contributions (for the reveal) then, and is nil otherwise. The
	// returned Match carries the current state either way.
	ConfirmContribution(ctx context.Context, itemID, matchID, contributionID string) (m Match, contribs []Contribution, completedNow bool, err error)
}

// DomainRepo persists custom domains within the bound tenant.
type DomainRepo interface {
	Create(ctx context.Context, d Domain) (Domain, error)
	// CreateReclaimingExpired inserts the domain, first reclaiming an existing claim
	// on the same hostname when that claim is unverified and was created before
	// expiredBefore — freeing a hostname parked by an unverified squatter (the
	// add-time namespace DoS, ADR-0004 §4). A verified claim is never reclaimed. The
	// check + reclaim + insert are atomic under a row lock, so two concurrent adds
	// cannot both reclaim. A zero expiredBefore disables reclaiming (behaves like
	// Create). Returns ErrConflict when the hostname is held by a verified or
	// still-within-window claim.
	CreateReclaimingExpired(ctx context.Context, d Domain, expiredBefore time.Time) (Domain, error)
	Get(ctx context.Context, id string) (Domain, error)
	List(ctx context.Context) ([]Domain, error)
	Update(ctx context.Context, d Domain) (Domain, error)
	Delete(ctx context.Context, id string) error
}
