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
}

// ListRepo persists lists within the bound tenant.
type ListRepo interface {
	Create(ctx context.Context, l List) (List, error)
	Get(ctx context.Context, id string) (List, error)
	// GetBySlug backs the public giver surface. It still resolves within the
	// bound tenant — the slug is unique per tenant.
	GetBySlug(ctx context.Context, shareSlug string) (List, error)
	List(ctx context.Context, ownerID string, p Page) ([]List, int, error)
	Update(ctx context.Context, l List) (List, error)
	Delete(ctx context.Context, id string) error
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

	// MarkReserverNotified moves active → reserver_notified, stamping decay_state_at
	// and the minted keep/release token hashes.
	MarkReserverNotified(ctx context.Context, reservationID string, at time.Time, keepTokenHash, releaseTokenHash string) (bool, error)
	// MarkExpired moves reserver_notified → expired, leaving the token hashes in
	// place (a late click resolves to 410, not 404).
	MarkExpired(ctx context.Context, reservationID string, at time.Time) (bool, error)
	// Renew moves reserver_notified → active on a "keep" click: it resets the decay
	// clock (last_activity_at, decay_state_at) and clears both one-click tokens.
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
}

// DomainRepo persists custom domains within the bound tenant.
type DomainRepo interface {
	Create(ctx context.Context, d Domain) (Domain, error)
	Get(ctx context.Context, id string) (Domain, error)
	List(ctx context.Context) ([]Domain, error)
	Update(ctx context.Context, d Domain) (Domain, error)
	Delete(ctx context.Context, id string) error
}
