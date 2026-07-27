// Package decay runs the reservation-decay flow. A reservation on a decaying list
// goes active → reserver_notified → expired; a "keep" click loops it back to
// active. The owner is never in the loop (they only set the per-list period).
// The whole time dimension is driven by an injected clock so it is testable with
// no real sleeps, and each transition is row-locked in storage — the email is
// sent only when the transition actually fires, so re-running the sweep (or
// racing a keep/release click) never double-emails. Anonymity is preserved
// throughout.
package decay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/settings"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// Config carries the instance-default knobs. Per-list overrides resolve through
// settings.Resolve; only the decay period has a per-list override today, but the
// response window flows through the same path.
type Config struct {
	// DefaultDecayDays is the instance-default decay period (0 = off) used when a
	// list does not override it.
	DefaultDecayDays int
	// ResponseWindow is how long the reserver has to click keep/release before the
	// reservation auto-expires.
	ResponseWindow time.Duration
	// LinkBase prefixes the keep/release one-click links in the reserver email
	// (…/decay-keep?token=RAW, …/decay-release?token=RAW).
	LinkBase string
}

// DefaultConfig ships decay off (period 0) with a 48h reserver response window.
func DefaultConfig() Config {
	return Config{DefaultDecayDays: 0, ResponseWindow: 48 * time.Hour}
}

// Sweeper advances due reservations one step per Sweep.
type Sweeper struct {
	store  storage.Store
	email  email.Sender
	clock  clock.Clock
	cfg    Config
	logger *slog.Logger
}

// NewSweeper builds a Sweeper.
func NewSweeper(store storage.Store, sender email.Sender, clk clock.Clock, cfg Config, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	if clk == nil {
		clk = clock.Real{}
	}
	return &Sweeper{store: store, email: sender, clock: clk, cfg: cfg, logger: logger}
}

// Sweep runs one idempotent pass over all decay candidates, advancing each at
// most one step. A failure on one candidate is logged and does not stop the rest.
func (s *Sweeper) Sweep(ctx context.Context) error {
	now := s.clock.Now()
	candidates, err := s.store.DecayCandidates(ctx)
	if err != nil {
		return err
	}
	for _, c := range candidates {
		if err := s.step(ctx, now, c); err != nil {
			s.logger.Error("decay step failed", "err", err, "reservation", c.ReservationID)
		}
	}
	return nil
}

func daysDuration(days int) time.Duration { return time.Duration(days) * 24 * time.Hour }

func (s *Sweeper) step(ctx context.Context, now time.Time, c storage.DecayCandidate) error {
	res := s.store.ForTenant(storage.Tenant{ID: c.TenantID}).Reservations()

	switch c.DecayState {
	case storage.DecayActive:
		// Effective period: list override if set, else the instance default. Decay
		// is on only when the resolved value is positive (never compare the raw
		// override).
		effectiveDays := settings.Resolve(c.DecayDays, s.cfg.DefaultDecayDays)
		if effectiveDays <= 0 || now.Sub(c.LastActivityAt) < daysDuration(effectiveDays) {
			return nil
		}
		keepRaw, keepHash, err := token.New()
		if err != nil {
			return err
		}
		releaseRaw, releaseHash, err := token.New()
		if err != nil {
			return err
		}
		// Gate the state-advance on a successful send: notify the reserver FIRST
		// and advance only if it went through, so a transient SMTP failure leaves
		// the reservation active (retried next sweep) rather than advancing and
		// later silently auto-expiring with the reserver never notified (#39).
		if err := s.notifyReserver(ctx, c, keepRaw, releaseRaw); err != nil {
			return nil // logged by notifyReserver; retried next sweep
		}
		// Send succeeded → advance and store the token hashes. MarkReserverNotified
		// stays the row-locked single-winner so the advance can't double. Rare
		// case: if this DB write fails after a successful send, the emailed links
		// are dead and the next sweep re-sends with fresh tokens — low harm,
		// intentional. (A future multi-instance sweeper could double-SEND before
		// one node advances; single-instance is clean.)
		if _, err := res.MarkReserverNotified(ctx, c.ReservationID, now, keepHash, releaseHash); err != nil {
			return err
		}

	case storage.DecayReserverNotified:
		window := settings.Resolve[time.Duration](nil, s.cfg.ResponseWindow)
		if now.Sub(c.DecayStateAt) < window {
			return nil
		}
		// Auto-release: expire (frees the item). No email — the owner is out of the
		// loop and the reserver was already notified.
		if _, err := res.MarkExpired(ctx, c.ReservationID, now); err != nil {
			return err
		}
	}
	return nil
}

// notifyReserver emails the reserver once with both one-click links, returning an
// error if it could not be sent — the caller gates the state-advance on this. A
// reservation with no reserver email can't be notified, so it is treated as a
// send failure (it stays active rather than advancing toward a silent expiry).
// The email names the item; no owner is contacted at any point.
func (s *Sweeper) notifyReserver(ctx context.Context, c storage.DecayCandidate, keepRaw, releaseRaw string) error {
	if c.GiverEmail == nil || *c.GiverEmail == "" {
		s.logger.Warn("decay: reserver has no email; not advancing", "reservation", c.ReservationID)
		return fmt.Errorf("decay: reservation %s has no reserver email", c.ReservationID)
	}
	keepLink := s.cfg.LinkBase + "/decay-keep?token=" + keepRaw
	releaseLink := s.cfg.LinkBase + "/decay-release?token=" + releaseRaw
	body := "You reserved " + c.ItemName + " a while ago — still planning to buy it? " +
		"Keep your reservation: " + keepLink + "  ·  Or release it for someone else: " + releaseLink
	if err := s.email.Send(ctx, email.Message{
		To:      *c.GiverEmail,
		Subject: "Still planning to buy your reserved gift?",
		Body:    body,
	}); err != nil {
		s.logger.Error("decay reserver email failed", "err", err, "reservation", c.ReservationID)
		return err
	}
	return nil
}
