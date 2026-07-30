package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type contributionRepo struct{ baseRepo }

const contributionCols = `id, tenant_id, item_id, pledged_amount_minor,
	pledged_currency, giver_name, contact_email, status, match_id, token_hash,
	match_action_token_hash, match_action_token_expires_at, created_at`

func scanContribution(s scanner) (storage.Contribution, error) {
	var (
		c            storage.Contribution
		giverName    sql.NullString
		matchID      sql.NullString
		actionExpiry sql.NullString
		createdAt    string
	)
	if err := s.Scan(&c.ID, &c.TenantID, &c.ItemID, &c.Pledged.AmountMinor,
		&c.Pledged.Currency, &giverName, &c.ContactEmail, &c.Status, &matchID,
		&c.TokenHash, &c.MatchActionTokenHash, &actionExpiry, &createdAt); err != nil {
		return storage.Contribution{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Contribution{}, err
	}
	if actionExpiry.Valid {
		exp, perr := parseTime(actionExpiry.String)
		if perr != nil {
			return storage.Contribution{}, perr
		}
		c.MatchActionTokenExpiresAt = &exp
	}
	c.GiverName = strPtr(giverName)
	c.MatchID = strPtr(matchID)
	c.CreatedAt = ts
	return c, nil
}

// prep fills server-set defaults and binds the tenant.
func (r contributionRepo) prep(c storage.Contribution) storage.Contribution {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.Status == "" {
		c.Status = storage.ContributionPending
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = nowTime()
	}
	c.TenantID = r.tenantID
	return c
}

// insert writes a prepared contribution via x (a *sql.DB or *sql.Tx).
func (r contributionRepo) insert(ctx context.Context, x execer, c storage.Contribution) error {
	_, err := x.ExecContext(ctx, r.rb(
		`INSERT INTO contributions (`+contributionCols+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.TenantID, c.ItemID, c.Pledged.AmountMinor, c.Pledged.Currency,
		nullStr(c.GiverName), c.ContactEmail, c.Status, nullStr(c.MatchID),
		c.TokenHash, c.MatchActionTokenHash, nullTime(c.MatchActionTokenExpiresAt),
		fmtTime(c.CreatedAt))
	return err
}

func (r contributionRepo) Create(ctx context.Context, c storage.Contribution) (storage.Contribution, error) {
	c = r.prep(c)
	if err := r.insert(ctx, r.db, c); err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.Contribution{}, storage.ErrConflict
		}
		return storage.Contribution{}, err
	}
	return c, nil
}

// CreateWithinCapacity atomically inserts a contribution only if the item's total
// non-terminal pledged amount would not exceed priceMinor; otherwise it returns
// ErrCapacityExceeded (ErrNotFound if the item is gone). Reuses the same item
// lock as reserve so contribute cannot overfund under concurrency.
func (r contributionRepo) CreateWithinCapacity(ctx context.Context, c storage.Contribution, priceMinor int64) (storage.Contribution, error) {
	c = r.prep(c)
	err := r.withItemLock(ctx, c.ItemID, func(tx *sql.Tx) error {
		// Reserve and co-buy are mutually exclusive per item (#93): an active
		// reservation (state != expired) takes the whole item, so no co-buy. Inside
		// the shared item lock, so a concurrent reserve can't slip in first.
		var reserved int
		if err := tx.QueryRowContext(ctx, r.rb(
			`SELECT COUNT(*) FROM reservations
			  WHERE tenant_id = ? AND item_id = ? AND state != 'expired'`),
			r.tenantID, c.ItemID).Scan(&reserved); err != nil {
			return err
		}
		if reserved > 0 {
			return storage.ErrCrossTrackConflict
		}
		var total int64
		if err := tx.QueryRowContext(ctx, r.rb(
			`SELECT COALESCE(SUM(pledged_amount_minor), 0) FROM contributions
			  WHERE tenant_id = ? AND item_id = ? AND status IN (?, ?, ?)`),
			r.tenantID, c.ItemID,
			string(storage.ContributionPending),
			string(storage.ContributionMatched),
			string(storage.ContributionConfirmed)).Scan(&total); err != nil {
			return err
		}
		if total+c.Pledged.AmountMinor > priceMinor {
			return storage.ErrCapacityExceeded
		}
		if err := r.insert(ctx, tx, c); err != nil {
			if r.d.isUniqueViolation(err) {
				return storage.ErrConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return storage.Contribution{}, err
	}
	return c, nil
}

func (r contributionRepo) get(ctx context.Context, cond, arg string) (storage.Contribution, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+contributionCols+` FROM contributions
		  WHERE tenant_id = ? AND `+cond), r.tenantID, arg)
	c, err := scanContribution(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Contribution{}, storage.ErrNotFound
		}
		return storage.Contribution{}, err
	}
	return c, nil
}

func (r contributionRepo) Get(ctx context.Context, id string) (storage.Contribution, error) {
	return r.get(ctx, `id = ?`, id)
}

func (r contributionRepo) ByTokenHash(ctx context.Context, tokenHash string) (storage.Contribution, error) {
	return r.get(ctx, `token_hash = ?`, tokenHash)
}

func (r contributionRepo) ListByItem(ctx context.Context, itemID string) ([]storage.Contribution, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+contributionCols+` FROM contributions
		  WHERE tenant_id = ? AND item_id = ?
		  ORDER BY created_at, id`), r.tenantID, itemID)
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

func (r contributionRepo) Update(ctx context.Context, c storage.Contribution) (storage.Contribution, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE contributions SET status = ?, match_id = ?
		  WHERE tenant_id = ? AND id = ?`),
		c.Status, nullStr(c.MatchID), r.tenantID, c.ID)
	if err != nil {
		return storage.Contribution{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.Contribution{}, err
	}
	return r.Get(ctx, c.ID)
}

func (r contributionRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM contributions WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}

// ByMatchActionTokenHash looks a contribution up by the hash of its scoped
// match-action token. An empty hash never matches (the default for contributions
// not in a proposed match), so it can't be used to fish for a row.
func (r contributionRepo) ByMatchActionTokenHash(ctx context.Context, tokenHash string) (storage.Contribution, error) {
	if tokenHash == "" {
		return storage.Contribution{}, storage.ErrNotFound
	}
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+contributionCols+` FROM contributions
		  WHERE tenant_id = ? AND match_action_token_hash = ?`), r.tenantID, tokenHash)
	c, err := scanContribution(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Contribution{}, storage.ErrNotFound
		}
		return storage.Contribution{}, err
	}
	return c, nil
}

// SetMatchActionToken installs (or, with an empty hash + nil expiry, clears) the
// scoped match-action token. It touches only those two columns, so it is safe to
// call alongside the match-linkage writes without disturbing status/match_id.
func (r contributionRepo) SetMatchActionToken(ctx context.Context, id, tokenHash string, expiresAt *time.Time) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE contributions SET match_action_token_hash = ?, match_action_token_expires_at = ?
		  WHERE tenant_id = ? AND id = ?`),
		tokenHash, nullTime(expiresAt), r.tenantID, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}
