package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type userRepo struct{ baseRepo }

func (r userRepo) Create(ctx context.Context, u storage.User) (storage.User, error) {
	if u.ID == "" {
		u.ID = newID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = nowTime()
	}
	u.TenantID = r.tenantID
	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO users (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)`),
		u.ID, u.TenantID, u.Name, fmtTime(u.CreatedAt))
	if err != nil {
		return storage.User{}, err
	}
	return u, nil
}

func (r userRepo) Get(ctx context.Context, id string) (storage.User, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, name, created_at
		   FROM users WHERE tenant_id = ? AND id = ?`), r.tenantID, id)

	var (
		u         storage.User
		createdAt string
	)
	if err := row.Scan(&u.ID, &u.TenantID, &u.Name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.User{}, storage.ErrNotFound
		}
		return storage.User{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.User{}, err
	}
	u.CreatedAt = ts
	return u, nil
}
