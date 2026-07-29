package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type reservationRepo struct{ baseRepo }

const reservationCols = `id, tenant_id, item_id, giver_name, giver_email,
	quantity, token_hash, created_at, last_activity_at, is_group, state,
	state_at, confirm_token_hash, email_confirmed_at, decay_release_token_hash,
	decay_keep_token_hash`

func scanReservation(s scanner) (storage.Reservation, error) {
	var (
		r            storage.Reservation
		giverName    sql.NullString
		giverEmail   sql.NullString
		createdAt    string
		lastActivity string
		isGroup      int
		stateAt      string
		confirmedAt  sql.NullString
	)
	if err := s.Scan(&r.ID, &r.TenantID, &r.ItemID, &giverName, &giverEmail,
		&r.Quantity, &r.TokenHash, &createdAt, &lastActivity, &isGroup, &r.State,
		&stateAt, &r.ConfirmTokenHash, &confirmedAt, &r.DecayReleaseTokenHash,
		&r.DecayKeepTokenHash); err != nil {
		return storage.Reservation{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Reservation{}, err
	}
	la, err := parseTime(lastActivity)
	if err != nil {
		return storage.Reservation{}, err
	}
	sa, err := parseTime(stateAt)
	if err != nil {
		return storage.Reservation{}, err
	}
	if confirmedAt.Valid && confirmedAt.String != "" {
		ca, err := parseTime(confirmedAt.String)
		if err != nil {
			return storage.Reservation{}, err
		}
		r.EmailConfirmedAt = &ca
	}
	r.GiverName = strPtr(giverName)
	r.GiverEmail = strPtr(giverEmail)
	r.CreatedAt = ts
	r.LastActivityAt = la
	r.StateAt = sa
	r.IsGroup = isGroup != 0
	return r, nil
}

// prep fills server-set defaults and binds the tenant.
func (r reservationRepo) prep(res storage.Reservation) storage.Reservation {
	if res.ID == "" {
		res.ID = newID()
	}
	if res.Quantity < 1 {
		res.Quantity = 1
	}
	if res.CreatedAt.IsZero() {
		res.CreatedAt = nowTime()
	}
	if res.LastActivityAt.IsZero() {
		res.LastActivityAt = res.CreatedAt
	}
	if res.State == "" {
		res.State = storage.StateActive
	}
	if res.StateAt.IsZero() {
		res.StateAt = res.CreatedAt
	}
	res.TenantID = r.tenantID
	return res
}

// insert writes a prepared reservation via x (a *sql.DB or *sql.Tx).
func (r reservationRepo) insert(ctx context.Context, x execer, res storage.Reservation) error {
	var confirmedAt any // nil → NULL
	if res.EmailConfirmedAt != nil {
		confirmedAt = fmtTime(*res.EmailConfirmedAt)
	}
	_, err := x.ExecContext(ctx, r.rb(
		`INSERT INTO reservations (`+reservationCols+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		res.ID, res.TenantID, res.ItemID, nullStr(res.GiverName), nullStr(res.GiverEmail),
		res.Quantity, res.TokenHash, fmtTime(res.CreatedAt),
		fmtTime(res.LastActivityAt), boolToInt(res.IsGroup), res.State,
		fmtTime(res.StateAt), res.ConfirmTokenHash, confirmedAt,
		res.DecayReleaseTokenHash, res.DecayKeepTokenHash)
	return err
}

func (r reservationRepo) Create(ctx context.Context, res storage.Reservation) (storage.Reservation, error) {
	res = r.prep(res)
	if err := r.insert(ctx, r.db, res); err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.Reservation{}, storage.ErrConflict
		}
		return storage.Reservation{}, err
	}
	return res, nil
}

func (r reservationRepo) CreateWithinCapacity(ctx context.Context, res storage.Reservation, wantedQty int) (storage.Reservation, error) {
	res = r.prep(res)
	err := r.withItemLock(ctx, res.ItemID, func(tx *sql.Tx) error {
		var total int
		if err := tx.QueryRowContext(ctx, r.rb(
			`SELECT COALESCE(SUM(quantity), 0) FROM reservations
			  WHERE tenant_id = ? AND item_id = ? AND state != 'expired'`),
			r.tenantID, res.ItemID).Scan(&total); err != nil {
			return err
		}
		if total+res.Quantity > wantedQty {
			return storage.ErrCapacityExceeded
		}
		if err := r.insert(ctx, tx, res); err != nil {
			if r.d.isUniqueViolation(err) {
				return storage.ErrConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return storage.Reservation{}, err
	}
	return res, nil
}

func (r reservationRepo) get(ctx context.Context, cond, arg string) (storage.Reservation, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+reservationCols+` FROM reservations
		  WHERE tenant_id = ? AND `+cond), r.tenantID, arg)
	res, err := scanReservation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Reservation{}, storage.ErrNotFound
		}
		return storage.Reservation{}, err
	}
	return res, nil
}

func (r reservationRepo) Get(ctx context.Context, id string) (storage.Reservation, error) {
	return r.get(ctx, `id = ?`, id)
}

func (r reservationRepo) ByTokenHash(ctx context.Context, tokenHash string) (storage.Reservation, error) {
	return r.get(ctx, `token_hash = ?`, tokenHash)
}

func (r reservationRepo) ListByItem(ctx context.Context, itemID string) ([]storage.Reservation, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+reservationCols+` FROM reservations
		  WHERE tenant_id = ? AND item_id = ?
		  ORDER BY created_at, id`), r.tenantID, itemID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.Reservation
	for rows.Next() {
		res, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

func (r reservationRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM reservations WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}
