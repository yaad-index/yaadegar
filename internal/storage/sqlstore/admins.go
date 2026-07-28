package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// EnsureAdmin upserts the instance-level superadmin by username: a fresh username
// is inserted, an existing one has its password hash refreshed (so rotating the
// configured hash takes effect on the next boot). Idempotent.
func (s *sqlStore) EnsureAdmin(ctx context.Context, username, passwordHash string) (storage.Admin, error) {
	if username == "" || passwordHash == "" {
		return storage.Admin{}, errors.New("sqlstore: admin username and password hash are required")
	}
	_, err := s.db.ExecContext(ctx, s.d.rebind(
		`INSERT INTO admins (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (username) DO UPDATE SET password_hash = excluded.password_hash`),
		newID(), username, passwordHash, fmtTime(nowTime()))
	if err != nil {
		return storage.Admin{}, err
	}
	return s.AdminByUsername(ctx, username)
}

// AdminByUsername looks up the superadmin by username.
func (s *sqlStore) AdminByUsername(ctx context.Context, username string) (storage.Admin, error) {
	return s.adminWhere(ctx, `username = ?`, username)
}

// AdminByID looks up the superadmin by id.
func (s *sqlStore) AdminByID(ctx context.Context, id string) (storage.Admin, error) {
	return s.adminWhere(ctx, `id = ?`, id)
}

func (s *sqlStore) adminWhere(ctx context.Context, cond, arg string) (storage.Admin, error) {
	row := s.db.QueryRowContext(ctx, s.d.rebind(
		`SELECT id, username, password_hash, created_at FROM admins WHERE `+cond), arg)
	var (
		a         storage.Admin
		createdAt string
	)
	if err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Admin{}, storage.ErrNotFound
		}
		return storage.Admin{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Admin{}, err
	}
	a.CreatedAt = ts
	return a, nil
}
