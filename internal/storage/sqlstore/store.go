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
	// SQLite is a single-writer database; pinning to one connection serializes
	// all access, which keeps the capacity checks race-free (no FOR UPDATE) and
	// avoids SQLITE_BUSY under concurrency.
	if cfg.Driver == storage.DriverSQLite {
		db.SetMaxOpenConns(1)
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
	if t.Subdomain == "" {
		return storage.Tenant{}, storage.ErrInvalidSubdomain
	}
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
		`SELECT t.id, t.subdomain, t.created_at, t.oauth_google_enabled
		   FROM tenants t
		   JOIN domains d ON d.tenant_id = t.id
		  WHERE d.hostname = ? AND d.verified = 1`), hostname)
	return scanTenant(row)
}

func (s *sqlStore) tenantWhere(ctx context.Context, cond string, arg any) (storage.Tenant, error) {
	row := s.db.QueryRowContext(ctx, s.d.rebind(
		`SELECT id, subdomain, created_at, oauth_google_enabled FROM tenants WHERE `+cond), arg)
	return scanTenant(row)
}

// SetTenantOAuthGoogle flips a tenant's Google-login toggle (ADR-0008 §6).
func (s *sqlStore) SetTenantOAuthGoogle(ctx context.Context, tenantID string, enabled bool) error {
	res, err := s.db.ExecContext(ctx, s.d.rebind(
		`UPDATE tenants SET oauth_google_enabled = ? WHERE id = ?`),
		boolToInt(enabled), tenantID)
	if err != nil {
		return err
	}
	return expectOne(res)
}

// ListTenants returns a page of all tenants (instance-admin browse, ADR-0009),
// oldest-first, with the unpaged total. Unscoped, the same class as CreateTenant.
func (s *sqlStore) ListTenants(ctx context.Context, p storage.Page) ([]storage.Tenant, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, s.d.rebind(
		`SELECT id, subdomain, created_at, oauth_google_enabled FROM tenants
		  ORDER BY created_at, id LIMIT ? OFFSET ?`), p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var tenants []storage.Tenant
	for rows.Next() {
		t, err := scanTenantRow(rows)
		if err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, t)
	}
	return tenants, total, rows.Err()
}

func scanTenant(row *sql.Row) (storage.Tenant, error) {
	t, err := scanTenantRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Tenant{}, storage.ErrNotFound
	}
	return t, err
}

func scanTenantRow(row rowScanner) (storage.Tenant, error) {
	var (
		t            storage.Tenant
		createdAt    string
		oauthEnabled int
	)
	if err := row.Scan(&t.ID, &t.Subdomain, &createdAt, &oauthEnabled); err != nil {
		return storage.Tenant{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Tenant{}, err
	}
	t.CreatedAt = ts
	t.OAuthGoogleEnabled = oauthEnabled != 0
	return t, nil
}
