package sqlstore

import (
	"context"
	"database/sql"
	"errors"

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
