package sqlstore_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// TestEveryServerSetTimestampComesFromTheStoreClock is #433. The clock is set
// years away from the wall clock, so a row stamped from time.Now cannot match it
// by accident. The rows are read back through a second connection rather than
// from what Create returns: the question is what reached the database.
func TestEveryServerSetTimestampComesFromTheStoreClock(t *testing.T) {
	ctx := context.Background()
	faked := time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)
	dsn := "file:" + filepath.Join(t.TempDir(), "clock.db")
	st, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn, Clock: clock.NewFake(faked)})
	require.NoError(t, err)
	require.NoError(t, st.Migrate(ctx))
	t.Cleanup(func() { _ = st.Close() })

	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)
	owner, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	list, err := ts.Lists().Create(ctx, storage.List{Title: "List", Active: true}, owner.ID)
	require.NoError(t, err)
	// AddOwner has its own stamp, separate from the one Create writes.
	require.NoError(t, ts.Lists().RemoveOwner(ctx, list.ID, owner.ID))
	require.NoError(t, ts.Lists().AddOwner(ctx, list.ID, owner.ID))
	item, err := ts.Items().Create(ctx, storage.Item{
		ListID: list.ID, Name: "Kettle", QuantityWanted: 2,
		Price: &storage.Money{AmountMinor: 4000, Currency: "EUR"},
	})
	require.NoError(t, err)
	_, err = ts.Reservations().Create(ctx, storage.Reservation{ItemID: item.ID, Quantity: 1, TokenHash: "r1"})
	require.NoError(t, err)
	c1, err := ts.Contributions().Create(ctx, storage.Contribution{
		ItemID: item.ID, Pledged: storage.Money{AmountMinor: 1000, Currency: "EUR"},
		ContactEmail: "one@example.com", TokenHash: "c1",
	})
	require.NoError(t, err)
	c2, err := ts.Contributions().Create(ctx, storage.Contribution{
		ItemID: item.ID, Pledged: storage.Money{AmountMinor: 1000, Currency: "EUR"},
		ContactEmail: "two@example.com", TokenHash: "c2",
	})
	require.NoError(t, err)
	_, err = ts.Matches().Create(ctx, storage.Match{ItemID: item.ID, ContributionIDs: []string{c1.ID, c2.ID}})
	require.NoError(t, err)
	_, err = ts.Domains().Create(ctx, storage.Domain{Hostname: "one.example.com", CNAMETarget: "alias.example"})
	require.NoError(t, err)
	_, err = ts.Domains().CreateReclaimingExpired(ctx, storage.Domain{Hostname: "two.example.com", CNAMETarget: "alias.example"}, time.Time{})
	require.NoError(t, err)
	_, err = ts.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID: owner.ID, Provider: storage.OAuthProviderGoogle, Subject: "sub", Email: "alice@example.com",
	})
	require.NoError(t, err)
	exp := faked.Add(time.Hour)
	_, err = ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{UserID: owner.ID, TokenHash: "p1", ExpiresAt: exp})
	require.NoError(t, err)
	_, err = ts.EmailVerificationTokens().Create(ctx, storage.EmailVerificationToken{UserID: owner.ID, TokenHash: "e1", ExpiresAt: exp})
	require.NoError(t, err)

	raw, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = raw.Close() })

	for _, col := range []struct{ table, column string }{
		{"schema_migrations", "applied_at"},
		{"tenants", "created_at"},
		{"users", "created_at"},
		{"lists", "created_at"},
		{"list_owners", "added_at"},
		{"items", "created_at"},
		{"reservations", "created_at"},
		{"reservations", "last_activity_at"},
		{"reservations", "state_at"},
		{"contributions", "created_at"},
		{"matches", "created_at"},
		{"domains", "created_at"},
		{"oauth_identities", "created_at"},
		{"password_reset_tokens", "created_at"},
		{"email_verification_tokens", "created_at"},
	} {
		t.Run(col.table+"."+col.column, func(t *testing.T) {
			rows, err := raw.QueryContext(ctx, `SELECT `+col.column+` FROM `+col.table)
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()
			n := 0
			for rows.Next() {
				var s string
				require.NoError(t, rows.Scan(&s))
				got, err := time.Parse(time.RFC3339Nano, s)
				require.NoError(t, err)
				assert.True(t, faked.Equal(got), "stamped %s, want the store clock's %s", s, faked.Format(time.RFC3339))
				n++
			}
			require.NoError(t, rows.Err())
			// An empty table would pass the loop above without checking anything.
			assert.Positive(t, n, "no rows: the setup above did not reach this table")
		})
	}
}

// TestTheStoreNeverReadsTheWallClock keeps #433 fixed: a new call site that
// stamps from time.Now compiles, passes every existing test, and puts back the
// second clock the test above is about.
func TestTheStoreNeverReadsTheWallClock(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		assert.NotContains(t, string(src), "time.Now(", "%s reads the wall clock; stamp from the store clock instead", f)
		checked++
	}
	assert.Positive(t, checked)
}
