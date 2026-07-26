package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type listRepo struct{ baseRepo }

const listCols = `id, tenant_id, owner_id, title, visibility, share_slug,
	event_date, decay_days, active, created_at`

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanList(s scanner) (storage.List, error) {
	var (
		l         storage.List
		eventDate sql.NullString
		active    int
		createdAt string
	)
	if err := s.Scan(&l.ID, &l.TenantID, &l.OwnerID, &l.Title, &l.Visibility,
		&l.ShareSlug, &eventDate, &l.DecayDays, &active, &createdAt); err != nil {
		return storage.List{}, err
	}
	ed, err := datePtr(eventDate)
	if err != nil {
		return storage.List{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.List{}, err
	}
	l.EventDate = ed
	l.Active = active != 0
	l.CreatedAt = ts
	return l, nil
}

func (r listRepo) Create(ctx context.Context, l storage.List) (storage.List, error) {
	if l.ID == "" {
		l.ID = newID()
	}
	if l.ShareSlug == "" {
		slug, err := newSlug()
		if err != nil {
			return storage.List{}, err
		}
		l.ShareSlug = slug
	}
	if l.Visibility == "" {
		l.Visibility = storage.VisibilityPrivate
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = nowTime()
	}
	l.TenantID = r.tenantID

	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO lists (`+listCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		l.ID, l.TenantID, l.OwnerID, l.Title, l.Visibility, l.ShareSlug,
		nullDate(l.EventDate), l.DecayDays, boolToInt(l.Active), fmtTime(l.CreatedAt))
	if err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.List{}, storage.ErrConflict
		}
		return storage.List{}, err
	}
	return l, nil
}

func (r listRepo) get(ctx context.Context, cond string, arg string) (storage.List, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+listCols+` FROM lists WHERE tenant_id = ? AND `+cond), r.tenantID, arg)
	l, err := scanList(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.List{}, storage.ErrNotFound
		}
		return storage.List{}, err
	}
	return l, nil
}

func (r listRepo) Get(ctx context.Context, id string) (storage.List, error) {
	return r.get(ctx, `id = ?`, id)
}

func (r listRepo) GetBySlug(ctx context.Context, shareSlug string) (storage.List, error) {
	return r.get(ctx, `share_slug = ?`, shareSlug)
}

func (r listRepo) List(ctx context.Context, ownerID string, p storage.Page) ([]storage.List, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM lists WHERE tenant_id = ? AND owner_id = ?`),
		r.tenantID, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+listCols+` FROM lists
		  WHERE tenant_id = ? AND owner_id = ?
		  ORDER BY created_at DESC, id
		  LIMIT ? OFFSET ?`),
		r.tenantID, ownerID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.List
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

func (r listRepo) Update(ctx context.Context, l storage.List) (storage.List, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE lists SET title = ?, visibility = ?, event_date = ?,
		        decay_days = ?, active = ?
		  WHERE tenant_id = ? AND id = ?`),
		l.Title, l.Visibility, nullDate(l.EventDate), l.DecayDays,
		boolToInt(l.Active), r.tenantID, l.ID)
	if err != nil {
		return storage.List{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.List{}, err
	}
	return r.Get(ctx, l.ID)
}

func (r listRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM lists WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}
