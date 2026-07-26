package sqlstore

import (
	"database/sql"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// tenantStore is a data-access handle bound to one tenant. The tenantID is
// private and captured at construction; every repository below stamps and
// filters on it, so no query can reach another tenant (ADR-0003 §2).
type tenantStore struct {
	db       *sql.DB
	d        dialect
	tenantID string
}

var (
	_ storage.TenantStore      = (*tenantStore)(nil)
	_ storage.UserRepo         = userRepo{}
	_ storage.ListRepo         = listRepo{}
	_ storage.ItemRepo         = itemRepo{}
	_ storage.ReservationRepo  = reservationRepo{}
	_ storage.ContributionRepo = contributionRepo{}
	_ storage.MatchRepo        = matchRepo{}
	_ storage.DomainRepo       = domainRepo{}
)

func (t *tenantStore) base() baseRepo {
	return baseRepo{db: t.db, d: t.d, tenantID: t.tenantID}
}

func (t *tenantStore) Users() storage.UserRepo                 { return userRepo{t.base()} }
func (t *tenantStore) Lists() storage.ListRepo                 { return listRepo{t.base()} }
func (t *tenantStore) Items() storage.ItemRepo                 { return itemRepo{t.base()} }
func (t *tenantStore) Reservations() storage.ReservationRepo   { return reservationRepo{t.base()} }
func (t *tenantStore) Contributions() storage.ContributionRepo { return contributionRepo{t.base()} }
func (t *tenantStore) Matches() storage.MatchRepo              { return matchRepo{t.base()} }
func (t *tenantStore) Domains() storage.DomainRepo             { return domainRepo{t.base()} }

// baseRepo carries the shared connection, dialect, and — crucially — the bound
// tenantID that every repository query uses.
type baseRepo struct {
	db       *sql.DB
	d        dialect
	tenantID string
}

// rb rebinds '?' placeholders for the active dialect.
func (b baseRepo) rb(q string) string { return b.d.rebind(q) }
