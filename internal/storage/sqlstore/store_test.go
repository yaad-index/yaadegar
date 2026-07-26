package sqlstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// newTestStore opens a migrated, throwaway SQLite database backed by a temp
// file (a file, not :memory:, so database/sql's pooled connections share one DB).
func newTestStore(t *testing.T) storage.Store {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db")
	st, err := sqlstore.Open(context.Background(), storage.Config{
		Driver: storage.DriverSQLite,
		DSN:    dsn,
	})
	require.NoError(t, err)
	require.NoError(t, st.Migrate(context.Background()))
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func mkTenant(t *testing.T, st storage.Store, subdomain string) storage.Tenant {
	t.Helper()
	ten, err := st.CreateTenant(context.Background(), storage.Tenant{Subdomain: subdomain})
	require.NoError(t, err)
	require.NotEmpty(t, ten.ID)
	return ten
}

func iptr(i int) *int { return &i }

func TestMigrate_Idempotent(t *testing.T) {
	st := newTestStore(t)
	// newTestStore already migrated once; a second call must be a no-op.
	require.NoError(t, st.Migrate(context.Background()))
	require.NoError(t, st.Ping(context.Background()))
}

func TestTenantLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	ten := mkTenant(t, st, "alice")

	byID, err := st.TenantByID(ctx, ten.ID)
	require.NoError(t, err)
	assert.Equal(t, ten.ID, byID.ID)
	assert.False(t, byID.CreatedAt.IsZero())

	bySub, err := st.TenantBySubdomain(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, ten.ID, bySub.ID)

	_, err = st.TenantBySubdomain(ctx, "nobody")
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// Duplicate subdomain conflicts.
	_, err = st.CreateTenant(ctx, storage.Tenant{Subdomain: "alice"})
	assert.ErrorIs(t, err, storage.ErrConflict)
}

func TestTenantByCustomDomain(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)

	// An unverified domain does not resolve.
	d, err := ts.Domains().Create(ctx, storage.Domain{
		Hostname:    "gifts.example.com",
		CNAMETarget: "alias.host.example",
		Verified:    false,
	})
	require.NoError(t, err)
	_, err = st.TenantByCustomDomain(ctx, "gifts.example.com")
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// Once verified, it resolves to the tenant.
	d.Verified = true
	_, err = ts.Domains().Update(ctx, d)
	require.NoError(t, err)
	got, err := st.TenantByCustomDomain(ctx, "gifts.example.com")
	require.NoError(t, err)
	assert.Equal(t, ten.ID, got.ID)
}

// TestTenantIsolation is the load-bearing test for ADR-0003 §2: one tenant's
// handle must never read or mutate another tenant's rows.
func TestTenantIsolation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	tenA := mkTenant(t, st, "alice")
	tenB := mkTenant(t, st, "bob")
	as := st.ForTenant(tenA)
	bs := st.ForTenant(tenB)

	owner, err := as.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	list, err := as.Lists().Create(ctx, storage.List{OwnerID: owner.ID, Title: "A's list"})
	require.NoError(t, err)
	item, err := as.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "A's item"})
	require.NoError(t, err)
	resv, err := as.Reservations().Create(ctx, storage.Reservation{ItemID: item.ID, TokenHash: "hash-a"})
	require.NoError(t, err)

	t.Run("reads are scoped", func(t *testing.T) {
		_, err := bs.Lists().Get(ctx, list.ID)
		assert.ErrorIs(t, err, storage.ErrNotFound)

		_, err = bs.Lists().GetBySlug(ctx, list.ShareSlug)
		assert.ErrorIs(t, err, storage.ErrNotFound)

		_, err = bs.Items().Get(ctx, item.ID)
		assert.ErrorIs(t, err, storage.ErrNotFound)

		_, err = bs.Reservations().ByTokenHash(ctx, "hash-a")
		assert.ErrorIs(t, err, storage.ErrNotFound)

		lists, total, err := bs.Lists().List(ctx, owner.ID, storage.Page{Limit: 100})
		require.NoError(t, err)
		assert.Empty(t, lists)
		assert.Zero(t, total)
	})

	t.Run("mutations are scoped", func(t *testing.T) {
		// B tries to update/delete A's rows: matches nothing, reports ErrNotFound.
		_, err := bs.Lists().Update(ctx, list)
		assert.ErrorIs(t, err, storage.ErrNotFound)
		assert.ErrorIs(t, bs.Lists().Delete(ctx, list.ID), storage.ErrNotFound)
		assert.ErrorIs(t, bs.Items().Delete(ctx, item.ID), storage.ErrNotFound)
		assert.ErrorIs(t, bs.Reservations().Delete(ctx, resv.ID), storage.ErrNotFound)

		// A's data is untouched.
		got, err := as.Lists().Get(ctx, list.ID)
		require.NoError(t, err)
		assert.Equal(t, "A's list", got.Title)
	})

	t.Run("same slug allowed across tenants", func(t *testing.T) {
		// share_slug is unique per tenant, not globally.
		ownerB, err := bs.Users().Create(ctx, storage.User{Name: "Bob"})
		require.NoError(t, err)
		_, err = bs.Lists().Create(ctx, storage.List{
			OwnerID:   ownerB.ID,
			Title:     "B's list",
			ShareSlug: list.ShareSlug,
		})
		require.NoError(t, err)
	})
}
