package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type contributionRepo struct{ baseRepo }

const contributionCols = `id, tenant_id, item_id, pledged_amount_minor,
	pledged_currency, giver_name, contact_email, status, match_id, token_hash, created_at`

func scanContribution(s scanner) (storage.Contribution, error) {
	var (
		c         storage.Contribution
		giverName sql.NullString
		matchID   sql.NullString
		createdAt string
	)
	if err := s.Scan(&c.ID, &c.TenantID, &c.ItemID, &c.Pledged.AmountMinor,
		&c.Pledged.Currency, &giverName, &c.ContactEmail, &c.Status, &matchID,
		&c.TokenHash, &createdAt); err != nil {
		return storage.Contribution{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Contribution{}, err
	}
	c.GiverName = strPtr(giverName)
	c.MatchID = strPtr(matchID)
	c.CreatedAt = ts
	return c, nil
}

func (r contributionRepo) Create(ctx context.Context, c storage.Contribution) (storage.Contribution, error) {
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

	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO contributions (`+contributionCols+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		c.ID, c.TenantID, c.ItemID, c.Pledged.AmountMinor, c.Pledged.Currency,
		nullStr(c.GiverName), c.ContactEmail, c.Status, nullStr(c.MatchID),
		c.TokenHash, fmtTime(c.CreatedAt))
	if err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.Contribution{}, storage.ErrConflict
		}
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
