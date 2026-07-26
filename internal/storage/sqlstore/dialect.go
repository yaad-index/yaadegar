package sqlstore

import (
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	sqlite "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"
)

// dialect captures the small differences between the databases we support. The
// CRUD body is otherwise shared (ADR-0003 §1): all queries are written with '?'
// placeholders and rebound per driver.
type dialect interface {
	// driverName is the database/sql driver to open.
	driverName() string
	// rebind rewrites '?' placeholders to the driver's parameter syntax.
	rebind(query string) string
	// migrationsSubdir is the embedded directory holding this dialect's SQL.
	migrationsSubdir() string
	// isUniqueViolation reports whether err is a unique-constraint conflict, so
	// repositories can translate it to storage.ErrConflict.
	isUniqueViolation(err error) bool
	// forUpdate is the row-locking clause appended to a SELECT to serialize
	// concurrent capacity checks on that row. Postgres uses "FOR UPDATE"; SQLite
	// has no such clause (it serializes writers via a single connection).
	forUpdate() string
}

type sqliteDialect struct{}

func (sqliteDialect) driverName() string       { return "sqlite" }
func (sqliteDialect) rebind(q string) string   { return q } // '?' is native
func (sqliteDialect) migrationsSubdir() string { return "migrations/sqlite" }
func (sqliteDialect) forUpdate() string        { return "" } // serialized via a single connection

func (sqliteDialect) isUniqueViolation(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		code := se.Code()
		return code == sqlitelib.SQLITE_CONSTRAINT_UNIQUE ||
			code == sqlitelib.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	// Fallback for wrapped/string-only errors.
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

type postgresDialect struct{}

func (postgresDialect) driverName() string       { return "pgx" }
func (postgresDialect) migrationsSubdir() string { return "migrations/postgres" }
func (postgresDialect) forUpdate() string        { return " FOR UPDATE" }

// rebind rewrites each '?' to a positional $N placeholder. Yaadegar never emits
// a literal '?' inside string literals, so a straight scan is safe.
func (postgresDialect) rebind(q string) string {
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

func (postgresDialect) isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == "23505" // unique_violation
	}
	return false
}
