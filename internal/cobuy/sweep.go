// Package cobuy runs the co-buying match auto-expiry sweep (#101). A match sits in
// `proposed` from the moment enough pledges cover an item until every party
// confirms (→ both_confirmed) or one declines/withdraws (→ dissolved). If nobody
// acts, the emailed confirm links expire after --cobuy-confirm-window (#96) but the
// match itself lingers — and post-#93 a lingering proposed match holds the item and
// blocks reserve too. This sweep completes the story symmetric with reservation
// decay: once the window elapses on a still-proposed match, it dissolves the match
// and drives every pledge terminal (expired), freeing the item for either track.
//
// The whole time dimension is driven by an injected clock so it is testable with no
// real sleeps, and the transition is row-locked and single-winner in storage
// (ExpireIfProposed) — so a sweep racing a confirm/decline never double-acts. No
// email is sent and no contact is read: the window already lapsed and anonymity is
// preserved throughout.
package cobuy

import (
	"context"
	"log/slog"
	"time"

	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// Sweeper expires stale proposed matches one pass at a time.
type Sweeper struct {
	store  storage.Store
	clock  clock.Clock
	window time.Duration
	logger *slog.Logger
}

// NewSweeper builds a Sweeper. window is the confirm window (the same value that
// governs the scoped-token expiry, #96); a non-positive window disables the sweep.
func NewSweeper(store storage.Store, clk clock.Clock, window time.Duration, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	if clk == nil {
		clk = clock.Real{}
	}
	return &Sweeper{store: store, clock: clk, window: window, logger: logger}
}

// Sweep runs one idempotent pass, dissolving every still-proposed match whose
// confirm window has elapsed. A failure on one match is logged and does not stop
// the rest. A non-positive window is a no-op (matches never auto-expire, matching
// the never-expiring scoped token).
func (s *Sweeper) Sweep(ctx context.Context) error {
	if s.window <= 0 {
		return nil
	}
	now := s.clock.Now()
	cutoff := now.Add(-s.window)
	candidates, err := s.store.ExpiredMatchCandidates(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, c := range candidates {
		matches := s.store.ForTenant(storage.Tenant{ID: c.TenantID}).Matches()
		if _, err := matches.ExpireIfProposed(ctx, c.ItemID, c.MatchID, now); err != nil {
			s.logger.Error("cobuy match expiry failed", "err", err, "match", c.MatchID)
		}
	}
	return nil
}
