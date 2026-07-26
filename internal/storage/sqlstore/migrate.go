package sqlstore

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationsFS embed.FS

// migration is one versioned SQL file. version is the filename's leading number
// (e.g. "0001"); it orders application and is recorded once applied.
type migration struct {
	version string
	name    string
	sql     string
}

// loadMigrations reads and orders the embedded SQL for a dialect.
func loadMigrations(d dialect) ([]migration, error) {
	dir := d.migrationsSubdir()
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %q: %w", dir, err)
	}
	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := migrationsFS.ReadFile(dir + "/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", e.Name(), err)
		}
		version, _, _ := strings.Cut(e.Name(), "_")
		ms = append(ms, migration{version: version, name: e.Name(), sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}

// migrate applies every pending migration in order. Each file runs in its own
// transaction and its version is recorded in schema_migrations; already-applied
// versions are skipped, so migrate is idempotent (ADR-0003 §4).
func migrate(ctx context.Context, db *sql.DB, d dialect) error {
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	ms, err := loadMigrations(d)
	if err != nil {
		return err
	}

	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if err := applyMigration(ctx, db, d, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func applyMigration(ctx context.Context, db *sql.DB, d dialect, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		d.rebind(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`),
		m.version, nowUTC(),
	); err != nil {
		return err
	}
	return tx.Commit()
}
