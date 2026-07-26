package sqlstore

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"time"

	"github.com/google/uuid"

	"github.com/yaad-index/yaadegar/internal/storage"
)

const dateLayout = "2006-01-02" // date-only columns (list event_date)

// nowTime is the canonical wall-clock used for server-set timestamps.
func nowTime() time.Time { return time.Now().UTC() }

// nowUTC is the canonical timestamp string written to TEXT columns.
func nowUTC() string { return nowTime().Format(time.RFC3339Nano) }

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

// newID mints an opaque server-generated id (ADR-0002 §9).
func newID() string { return uuid.NewString() }

// newSlug mints an opaque, unguessable share slug (ADR-0002 §9): 16 random
// bytes, URL-safe base64, no padding.
func newSlug() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// nullStr maps an optional string to a nullable column value.
func nullStr(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// strPtr maps a nullable column value back to an optional string.
func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// nullDate maps an optional date to a nullable date-only column value.
func nullDate(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.UTC().Format(dateLayout), Valid: true}
}

// datePtr maps a nullable date-only column back to an optional date.
func datePtr(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// priceCols maps an optional Money to its two nullable columns.
func priceCols(m *storage.Money) (sql.NullInt64, sql.NullString) {
	if m == nil {
		return sql.NullInt64{}, sql.NullString{}
	}
	return sql.NullInt64{Int64: m.AmountMinor, Valid: true},
		sql.NullString{String: m.Currency, Valid: true}
}

// pricePtr maps the two nullable price columns back to an optional Money. A
// present currency marks a present price.
func pricePtr(amount sql.NullInt64, currency sql.NullString) *storage.Money {
	if !currency.Valid {
		return nil
	}
	return &storage.Money{AmountMinor: amount.Int64, Currency: currency.String}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// expectOne turns a zero-rows-affected result into ErrNotFound, so an update or
// delete that matched nothing within the tenant scope is reported uniformly.
func expectOne(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}
