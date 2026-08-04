package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/preview"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// blockingResetStore wraps a real Store and parks every password-reset-token Create
// until release is closed, signalling entered when the call is reached. It lets a
// test hold the token INSERT open and observe whether the request path waits on it.
// Only PasswordResetTokens().Create is affected; every other operation delegates
// straight through, so seeding and reads are unblocked.
type blockingResetStore struct {
	storage.Store
	entered chan struct{}
	release chan struct{}
	once    *sync.Once
}

func (s blockingResetStore) ForTenant(t storage.Tenant) storage.TenantStore {
	return blockingResetTenant{s.Store.ForTenant(t), s.entered, s.release, s.once}
}

type blockingResetTenant struct {
	storage.TenantStore
	entered chan struct{}
	release chan struct{}
	once    *sync.Once
}

func (t blockingResetTenant) PasswordResetTokens() storage.PasswordResetTokenRepo {
	return blockingResetRepo{t.TenantStore.PasswordResetTokens(), t.entered, t.release, t.once}
}

type blockingResetRepo struct {
	storage.PasswordResetTokenRepo
	entered chan struct{}
	release chan struct{}
	once    *sync.Once
}

func (r blockingResetRepo) Create(ctx context.Context, tok storage.PasswordResetToken) (storage.PasswordResetToken, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.PasswordResetTokenRepo.Create(ctx, tok)
}

// TestPasswordResetRequestPersistsOffResponsePath proves the request path does NO
// found-only synchronous DB work (#159): the token persist runs in the async worker,
// not on the response path. The reset-token Create is parked open; a found request
// still returns 202 promptly (it would deadlock if the INSERT were synchronous), and
// an unknown identifier returns the same 202 having never reached Create at all — so
// found and not-found do equivalent synchronous work. Persist strictly precedes send:
// while Create is parked, no email has gone out; releasing it lets the mail through.
func TestPasswordResetRequestPersistsOffResponsePath(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "api.db")
	real, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn})
	require.NoError(t, err)
	require.NoError(t, real.Migrate(ctx))
	t.Cleanup(func() { _ = real.Close() })

	tenant, err := real.CreateTenant(ctx, storage.Tenant{Subdomain: "alice"})
	require.NoError(t, err)

	entered := make(chan struct{})
	release := make(chan struct{})
	wrapped := blockingResetStore{Store: real, entered: entered, release: release, once: &sync.Once{}}

	fake := &email.FakeSender{}
	clk := clock.NewFake(testClockStart)
	authSvc, err := auth.NewService(auth.Config{JWTSecret: testJWTSecret, PasswordEnabled: true}, clk)
	require.NoError(t, err)
	handler := api.NewHandler(wrapped, api.Options{
		BaseDomain:        baseDomain,
		Logger:            slog.New(slog.DiscardHandler),
		Email:             fake,
		Clock:             clk,
		Previewer:         preview.New(&preview.FakeFetcher{}),
		Resolver:          &fakeResolver{txt: map[string][]string{}},
		Auth:              authSvc,
		DomainCNAMETarget: "cname.yaadegar.test",
		DomainClaimTTL:    testDomainClaimTTL,
	})
	// The reset-token Create is blocked, but seeding creates a user (not a token), so
	// h.store is the real store for unblocked seeding and reads.
	h := &harness{t: t, h: handler, store: real, tenant: tenant, email: fake, clk: clk, authSvc: authSvc}
	h.seedOwnerWithEmail("erin", "erin@example.com", "first-password")

	// A found request returns 202 without waiting for the parked token INSERT — the
	// persist is off the response path. Run it in a goroutine so a regression (a
	// synchronous INSERT) surfaces as a timeout here rather than hanging the test.
	found := make(chan int, 1)
	go func() {
		resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
			map[string]any{"identifier": "erin@example.com"})
		found <- resp.StatusCode
	}()
	select {
	case code := <-found:
		require.Equal(t, http.StatusAccepted, code)
	case <-time.After(2 * time.Second):
		t.Fatal("found request blocked on the token persist — the INSERT is still on the response path")
	}

	// The async worker reached the persist step (proves it is attempted off-path) and
	// is now parked before the send.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("async token persist was never attempted")
	}

	// An unknown identifier returns the same 202 and never reaches Create at all — so
	// the two paths do equivalent synchronous work (no found-only INSERT).
	resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
		map[string]any{"identifier": "ghost"})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Empty(t, body)

	// Persist strictly precedes send: while the INSERT is parked, no mail has gone out.
	assert.Empty(t, fake.Sent(), "no email until the token is persisted")

	// Release the persist; the emailed token now goes out (persist→send completed).
	close(release)
	assert.Eventually(t, func() bool { return len(fake.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, "erin@example.com", fake.Sent()[0].To)
}
