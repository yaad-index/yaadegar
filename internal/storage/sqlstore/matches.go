package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type matchRepo struct{ baseRepo }

// Create inserts the match and its contribution links atomically, and stamps
// each linked contribution with match_id + status=matched.
func (r matchRepo) Create(ctx context.Context, m storage.Match) (storage.Match, error) {
	if m.ID == "" {
		m.ID = newID()
	}
	if m.State == "" {
		m.State = storage.MatchProposed
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = nowTime()
	}
	m.TenantID = r.tenantID

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Match{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.rb(
		`INSERT INTO matches (id, tenant_id, item_id, state, created_at)
		 VALUES (?, ?, ?, ?, ?)`),
		m.ID, m.TenantID, m.ItemID, m.State, fmtTime(m.CreatedAt)); err != nil {
		return storage.Match{}, err
	}
	for _, cid := range m.ContributionIDs {
		if _, err := tx.ExecContext(ctx, r.rb(
			`INSERT INTO match_contributions (tenant_id, match_id, contribution_id)
			 VALUES (?, ?, ?)`), m.TenantID, m.ID, cid); err != nil {
			return storage.Match{}, err
		}
		if _, err := tx.ExecContext(ctx, r.rb(
			`UPDATE contributions SET match_id = ?, status = ?
			  WHERE tenant_id = ? AND id = ?`),
			m.ID, storage.ContributionMatched, m.TenantID, cid); err != nil {
			return storage.Match{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return storage.Match{}, err
	}
	return m, nil
}

func (r matchRepo) contributionIDs(ctx context.Context, matchID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT contribution_id FROM match_contributions
		  WHERE tenant_id = ? AND match_id = ?
		  ORDER BY contribution_id`), r.tenantID, matchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r matchRepo) scanMatch(ctx context.Context, s scanner) (storage.Match, error) {
	var (
		m         storage.Match
		createdAt string
	)
	if err := s.Scan(&m.ID, &m.TenantID, &m.ItemID, &m.State, &createdAt); err != nil {
		return storage.Match{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Match{}, err
	}
	m.CreatedAt = ts
	ids, err := r.contributionIDs(ctx, m.ID)
	if err != nil {
		return storage.Match{}, err
	}
	m.ContributionIDs = ids
	return m, nil
}

func (r matchRepo) Get(ctx context.Context, id string) (storage.Match, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, item_id, state, created_at FROM matches
		  WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	m, err := r.scanMatch(ctx, row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Match{}, storage.ErrNotFound
		}
		return storage.Match{}, err
	}
	return m, nil
}

func (r matchRepo) ListByItem(ctx context.Context, itemID string) ([]storage.Match, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT id, tenant_id, item_id, state, created_at FROM matches
		  WHERE tenant_id = ? AND item_id = ?
		  ORDER BY created_at, id`), r.tenantID, itemID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var matches []storage.Match
	for rows.Next() {
		// scanMatch issues a follow-up query, so materialize the base rows first.
		var (
			m         storage.Match
			createdAt string
		)
		if err := rows.Scan(&m.ID, &m.TenantID, &m.ItemID, &m.State, &createdAt); err != nil {
			return nil, err
		}
		ts, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		m.CreatedAt = ts
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range matches {
		ids, err := r.contributionIDs(ctx, matches[i].ID)
		if err != nil {
			return nil, err
		}
		matches[i].ContributionIDs = ids
	}
	return matches, nil
}

func (r matchRepo) Update(ctx context.Context, m storage.Match) (storage.Match, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE matches SET state = ? WHERE tenant_id = ? AND id = ?`),
		m.State, r.tenantID, m.ID)
	if err != nil {
		return storage.Match{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.Match{}, err
	}
	return r.Get(ctx, m.ID)
}

// ConfirmContribution runs the confirm transition atomically under the item lock:
// mark the contribution confirmed, re-read the match's contributions, and — only
// if the match is still proposed and all are now confirmed — flip it to
// both_confirmed. The lock (FOR UPDATE on Postgres; a single connection on SQLite)
// serializes concurrent confirms, so exactly one call observes the completing
// transition and reports completedNow=true, and the reveal fires exactly once (#36).
func (r matchRepo) ConfirmContribution(ctx context.Context, itemID, matchID, contributionID string) (storage.Match, []storage.Contribution, bool, error) {
	var (
		contribs     []storage.Contribution
		completedNow bool
	)
	err := r.withItemLock(ctx, itemID, func(tx *sql.Tx) error {
		// Read the match state under the lock. An already-resolved match (a concurrent
		// confirm or decline got here first) takes no further transition.
		var state string
		if err := tx.QueryRowContext(ctx, r.rb(
			`SELECT state FROM matches WHERE tenant_id = ? AND id = ?`),
			r.tenantID, matchID).Scan(&state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return storage.ErrNotFound
			}
			return err
		}
		if storage.MatchState(state) != storage.MatchProposed {
			return nil
		}
		// Mark this contribution confirmed (scoped to the match as a guard).
		res, err := tx.ExecContext(ctx, r.rb(
			`UPDATE contributions SET status = ?
			  WHERE tenant_id = ? AND id = ? AND match_id = ?`),
			storage.ContributionConfirmed, r.tenantID, contributionID, matchID)
		if err != nil {
			return err
		}
		if err := expectOne(res); err != nil {
			return err
		}
		// Re-read the match's contributions and check for full confirmation.
		cs, err := r.contributionsByMatchTx(ctx, tx, matchID)
		if err != nil {
			return err
		}
		allConfirmed := len(cs) > 0
		for _, c := range cs {
			if c.Status != storage.ContributionConfirmed {
				allConfirmed = false
				break
			}
		}
		if allConfirmed {
			if _, err := tx.ExecContext(ctx, r.rb(
				`UPDATE matches SET state = ? WHERE tenant_id = ? AND id = ?`),
				storage.MatchBothConfirmed, r.tenantID, matchID); err != nil {
				return err
			}
			completedNow = true
			contribs = cs
		}
		return nil
	})
	if err != nil {
		return storage.Match{}, nil, false, err
	}
	// The match's current (committed) state, for the caller's response.
	m, err := r.Get(ctx, matchID)
	if err != nil {
		return storage.Match{}, nil, false, err
	}
	return m, contribs, completedNow, nil
}

// ExpireIfProposed tears down a still-proposed match under the item lock: match →
// expired, all its contributions → expired (terminal, freeing the item) with their
// scoped tokens cleared. The from-state guard under the lock makes it a
// single-winner, idempotent no-op (moved=false) when a concurrent confirm/decline
// already resolved the match (#101).
func (r matchRepo) ExpireIfProposed(ctx context.Context, itemID, matchID string, at time.Time) (bool, error) {
	moved := false
	err := r.withItemLock(ctx, itemID, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx, r.rb(
			`SELECT state FROM matches WHERE tenant_id = ? AND id = ?`),
			r.tenantID, matchID).Scan(&state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return storage.ErrNotFound
			}
			return err
		}
		if storage.MatchState(state) != storage.MatchProposed {
			return nil
		}
		// Every pledge on the match goes terminal; the scoped match-action tokens are
		// spent, so clear them. match_id is left in place as the audit link.
		if _, err := tx.ExecContext(ctx, r.rb(
			`UPDATE contributions
			    SET status = ?, match_action_token_hash = '', match_action_token_expires_at = NULL
			  WHERE tenant_id = ? AND match_id = ?`),
			storage.ContributionExpired, r.tenantID, matchID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.rb(
			`UPDATE matches SET state = ? WHERE tenant_id = ? AND id = ?`),
			storage.MatchExpired, r.tenantID, matchID); err != nil {
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

// contributionsByMatchTx reads a match's contributions within the given tx.
func (r matchRepo) contributionsByMatchTx(ctx context.Context, tx *sql.Tx, matchID string) ([]storage.Contribution, error) {
	rows, err := tx.QueryContext(ctx, r.rb(
		`SELECT `+contributionCols+` FROM contributions
		  WHERE tenant_id = ? AND match_id = ?
		  ORDER BY created_at, id`), r.tenantID, matchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.Contribution
	for rows.Next() {
		c, err := scanContribution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
