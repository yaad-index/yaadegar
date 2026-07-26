//go:build integration

// Package sqlstore's Postgres integration test. It is guarded by the
// `integration` build tag so the default `check` CI (which has no database)
// stays green; run it explicitly against a live Postgres:
//
//	YAADEGAR_TEST_POSTGRES_DSN='postgres://user:pass@localhost:5432/yaadegar_test?sslmode=disable' \
//	    go test -tags=integration -race ./internal/storage/sqlstore/
//
// The Postgres driver shares the entire CRUD body with SQLite (ADR-0003 §1); this
// test exists to prove the Postgres dialect, placeholder rebinding, and migration
// SQL actually run against a real server.
package sqlstore_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

func newPostgresStore(t *testing.T) storage.Store {
	t.Helper()
	dsn := os.Getenv("YAADEGAR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set YAADEGAR_TEST_POSTGRES_DSN to run the Postgres integration test")
	}
	st, err := sqlstore.Open(context.Background(), storage.Config{
		Driver: storage.DriverPostgres,
		DSN:    dsn,
	})
	require.NoError(t, err)
	require.NoError(t, st.Migrate(context.Background()))
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestPostgres_RoundTripAndIsolation runs a representative slice of the contract
// against a real Postgres: a scoped CRUD round-trip, unique-conflict mapping, and
// the cross-tenant isolation guarantee.
func TestPostgres_RoundTripAndIsolation(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)

	// Unique subdomains per run to tolerate a shared, non-reset database.
	suffix := t.Name()
	tenA, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "a-" + suffix})
	require.NoError(t, err)
	tenB, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "b-" + suffix})
	require.NoError(t, err)

	// Duplicate subdomain → ErrConflict (exercises pgconn 23505 mapping).
	_, err = st.CreateTenant(ctx, storage.Tenant{Subdomain: "a-" + suffix})
	assert.ErrorIs(t, err, storage.ErrConflict)

	as := st.ForTenant(tenA)
	bs := st.ForTenant(tenB)

	owner, err := as.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	list, err := as.Lists().Create(ctx, storage.List{
		OwnerID: owner.ID,
		Title:   "PG list",
	})
	require.NoError(t, err)
	item, err := as.Items().Create(ctx, storage.Item{
		ListID: list.ID,
		Name:   "Kettle",
		Price:  &storage.Money{AmountMinor: 3500, Currency: "GBP"},
	})
	require.NoError(t, err)

	got, err := as.Items().Get(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Price)
	assert.Equal(t, int64(3500), got.Price.AmountMinor)

	// Cross-tenant isolation holds on Postgres too.
	_, err = bs.Lists().Get(ctx, list.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)
	_, err = bs.Items().Get(ctx, item.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}
