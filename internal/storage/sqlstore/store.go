// Package sqlstore is the database/sql-backed implementation of the storage
// interfaces. One CRUD body serves both SQLite and Postgres; a dialect abstracts
// the few differences (ADR-0003 §1). All domain access is tenant-scoped by
// construction — see storage.Store and ADR-0003 §2.
package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	// Registers the pure-Go "sqlite" database/sql driver.
	_ "modernc.org/sqlite"
	// Registers the "pgx" database/sql driver.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// sqlStore is the top-level handle. It is safe for concurrent use.
type sqlStore struct {
	db *sql.DB
	d  dialect
}

var _ storage.Store = (*sqlStore)(nil)

func dialectFor(driver storage.Driver) (dialect, error) {
	switch driver {
	case storage.DriverSQLite:
		return sqliteDialect{}, nil
	case storage.DriverPostgres:
		return postgresDialect{}, nil
	default:
		return nil, fmt.Errorf("sqlstore: unknown driver %q", driver)
	}
}

// Open connects to the configured database and returns a Store. It does not run
// migrations; call Store.Migrate for that.
func Open(ctx context.Context, cfg storage.Config) (storage.Store, error) {
	d, err := dialectFor(cfg.Driver)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(d.driverName(), cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: open %s: %w", cfg.Driver, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlstore: connect %s: %w", cfg.Driver, err)
	}
	return &sqlStore{db: db, d: d}, nil
}

func (s *sqlStore) Migrate(ctx context.Context) error { return migrate(ctx, s.db, s.d) }

func (s *sqlStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *sqlStore) Close() error { return s.db.Close() }

// ForTenant returns a data-access handle bound to t. Every repository it hands
// out filters and stamps tenant_id from t.ID (ADR-0003 §2).
func (s *sqlStore) ForTenant(t storage.Tenant) storage.TenantStore {
	return &tenantStore{db: s.db, d: s.d, tenantID: t.ID}
}

func (s *sqlStore) CreateTenant(ctx context.Context, t storage.Tenant) (storage.Tenant, error) {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = nowTime()
	}
	_, err := s.db.ExecContext(ctx,
		s.d.rebind(`INSERT INTO tenants (id, subdomain, created_at) VALUES (?, ?, ?)`),
		t.ID, t.Subdomain, fmtTime(t.CreatedAt),
	)
	if err != nil {
		if s.d.isUniqueViolation(err) {
			return storage.Tenant{}, storage.ErrConflict
		}
		return storage.Tenant{}, err
	}
	return t, nil
}

func (s *sqlStore) TenantByID(ctx context.Context, id string) (storage.Tenant, error) {
	return s.tenantWhere(ctx, `id = ?`, id)
}

func (s *sqlStore) TenantBySubdomain(ctx context.Context, subdomain string) (storage.Tenant, error) {
	return s.tenantWhere(ctx, `subdomain = ?`, subdomain)
}

// TenantByCustomDomain resolves a verified custom hostname to its tenant.
func (s *sqlStore) TenantByCustomDomain(ctx context.Context, hostname string) (storage.Tenant, error) {
	row := s.db.QueryRowContext(ctx, s.d.rebind(
		`SELECT t.id, t.subdomain, t.created_at
		   FROM tenants t
		   JOIN domains d ON d.tenant_id = t.id
		  WHERE d.hostname = ? AND d.verified = 1`), hostname)
	return scanTenant(row)
}

func (s *sqlStore) tenantWhere(ctx context.Context, cond string, arg any) (storage.Tenant, error) {
	row := s.db.QueryRowContext(ctx, s.d.rebind(
		`SELECT id, subdomain, created_at FROM tenants WHERE `+cond), arg)
	return scanTenant(row)
}

func scanTenant(row *sql.Row) (storage.Tenant, error) {
	var (
		t         storage.Tenant
		createdAt string
	)
	if err := row.Scan(&t.ID, &t.Subdomain, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Tenant{}, storage.ErrNotFound
		}
		return storage.Tenant{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Tenant{}, err
	}
	t.CreatedAt = ts
	return t, nil
}
