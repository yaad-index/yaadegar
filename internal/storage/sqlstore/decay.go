package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// ByDecayReleaseTokenHash / ByDecayKeepTokenHash look a reservation up by the hash
// of its one-click release / keep token. Empty hashes never match (the column
// default for reservations without minted tokens).
func (r reservationRepo) ByDecayReleaseTokenHash(ctx context.Context, tokenHash string) (storage.Reservation, error) {
	if tokenHash == "" {
		return storage.Reservation{}, storage.ErrNotFound
	}
	return r.get(ctx, `decay_release_token_hash = ?`, tokenHash)
}

func (r reservationRepo) ByDecayKeepTokenHash(ctx context.Context, tokenHash string) (storage.Reservation, error) {
	if tokenHash == "" {
		return storage.Reservation{}, storage.ErrNotFound
	}
	return r.get(ctx, `decay_keep_token_hash = ?`, tokenHash)
}

// transitionState is the shared row-locked, idempotent decay transition: it moves
// decay_state from `from` to `to` only if the current state is still `from`,
// applying update() inside the same transaction. It returns false (no error) when
// the state already moved — so concurrent sweeps/keeps/releases are safe no-ops.
func (r reservationRepo) transitionState(ctx context.Context, id string, from, to storage.ReservationDecayState, update string, args ...any) (bool, error) {
	moved := false
	err := r.withRowLock(ctx, "reservations", id, func(tx *sql.Tx) error {
		var current storage.ReservationDecayState
		if err := tx.QueryRowContext(ctx, r.rb(
			`SELECT decay_state FROM reservations WHERE tenant_id = ? AND id = ?`),
			r.tenantID, id).Scan(&current); err != nil {
			return err
		}
		if current != from {
			return nil
		}
		if _, err := tx.ExecContext(ctx, r.rb(update), append(args, r.tenantID, id)...); err != nil {
			return err
		}
		moved = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return moved, nil
}

// MarkReserverNotified moves active → reserver_notified, stamping decay_state_at
// and the freshly-minted keep/release token hashes.
func (r reservationRepo) MarkReserverNotified(ctx context.Context, id string, at time.Time, keepHash, releaseHash string) (bool, error) {
	return r.transitionState(ctx, id, storage.DecayActive, storage.DecayReserverNotified,
		`UPDATE reservations SET decay_state = ?, decay_state_at = ?,
		        decay_keep_token_hash = ?, decay_release_token_hash = ?
		  WHERE tenant_id = ? AND id = ?`,
		storage.DecayReserverNotified, fmtTime(at), keepHash, releaseHash)
}

// MarkExpired moves reserver_notified → expired. The token hashes are left in
// place so a late keep/release click can resolve to a 410 rather than a 404.
func (r reservationRepo) MarkExpired(ctx context.Context, id string, at time.Time) (bool, error) {
	return r.transitionState(ctx, id, storage.DecayReserverNotified, storage.DecayExpired,
		`UPDATE reservations SET decay_state = ?, decay_state_at = ?
		  WHERE tenant_id = ? AND id = ?`,
		storage.DecayExpired, fmtTime(at))
}

// Renew moves reserver_notified → active (the reserver clicked "keep"): it resets
// the decay clock (last_activity_at and decay_state_at) and invalidates both
// one-click tokens so a fresh pair is minted next cycle.
func (r reservationRepo) Renew(ctx context.Context, id string, at time.Time) (bool, error) {
	return r.transitionState(ctx, id, storage.DecayReserverNotified, storage.DecayActive,
		`UPDATE reservations SET decay_state = ?, decay_state_at = ?, last_activity_at = ?,
		        decay_keep_token_hash = '', decay_release_token_hash = ''
		  WHERE tenant_id = ? AND id = ?`,
		storage.DecayActive, fmtTime(at), fmtTime(at))
}

// DecayCandidates returns non-expired reservations on active lists across all
// tenants, joined with the item name and the list's decay-period override (nil =
// inherit the instance default). Trusted system read; the sweeper resolves the
// effective period and never compares the raw override.
func (s *sqlStore) DecayCandidates(ctx context.Context) ([]storage.DecayCandidate, error) {
	rows, err := s.db.QueryContext(ctx, s.d.rebind(
		`SELECT r.tenant_id, r.id, r.item_id, i.name, r.giver_email,
		        r.decay_state, r.last_activity_at, r.decay_state_at, l.decay_days
		   FROM reservations r
		   JOIN items i ON i.tenant_id = r.tenant_id AND i.id = r.item_id
		   JOIN lists l ON l.tenant_id = r.tenant_id AND l.id = i.list_id
		  WHERE r.decay_state != ? AND l.active = 1
		  ORDER BY r.tenant_id, r.id`),
		string(storage.DecayExpired))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.DecayCandidate
	for rows.Next() {
		var (
			c            storage.DecayCandidate
			giverEmail   sql.NullString
			lastActivity string
			decayStateAt string
			decayDays    int
		)
		if err := rows.Scan(&c.TenantID, &c.ReservationID, &c.ItemID, &c.ItemName,
			&giverEmail, &c.DecayState, &lastActivity, &decayStateAt, &decayDays); err != nil {
			return nil, err
		}
		la, err := parseTime(lastActivity)
		if err != nil {
			return nil, err
		}
		dsa, err := parseTime(decayStateAt)
		if err != nil {
			return nil, err
		}
		c.GiverEmail = strPtr(giverEmail)
		c.LastActivityAt = la
		c.DecayStateAt = dsa
		c.DecayDays = decayDaysFromStorage(decayDays)
		out = append(out, c)
	}
	return out, rows.Err()
}
