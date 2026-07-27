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
		`INSERT INTO users (id, tenant_id, name, email, username, password_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		u.ID, u.TenantID, u.Name, u.Email, usernameArg(u.Username), u.PasswordHash, fmtTime(u.CreatedAt))
	if err != nil {
		return storage.User{}, err
	}
	return u, nil
}

func (r userRepo) Get(ctx context.Context, id string) (storage.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, name, email, username, password_hash, created_at
		   FROM users WHERE tenant_id = ? AND id = ?`), r.tenantID, id))
}

func (r userRepo) ByUsername(ctx context.Context, username string) (storage.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, name, email, username, password_hash, created_at
		   FROM users WHERE tenant_id = ? AND username = ?`), r.tenantID, username))
}

func (r userRepo) scanUser(row *sql.Row) (storage.User, error) {
	var (
		u         storage.User
		username  sql.NullString
		createdAt string
	)
	if err := row.Scan(&u.ID, &u.TenantID, &u.Name, &u.Email, &username, &u.PasswordHash, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.User{}, storage.ErrNotFound
		}
		return storage.User{}, err
	}
	if username.Valid {
		u.Username = &username.String
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.User{}, err
	}
	u.CreatedAt = ts
	return u, nil
}

// usernameArg maps an optional username to a NULL-able driver value, so
// credential-less users store NULL (not ”) and the partial unique index permits
// many of them per tenant.
func usernameArg(username *string) any {
	if username == nil || *username == "" {
		return nil
	}
	return *username
}
