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
	if u.Role == "" {
		u.Role = storage.RoleOwner
	}
	u.TenantID = r.tenantID
	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO users (id, tenant_id, name, email, username, password_hash, role, banned, is_admin, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, u.TenantID, u.Name, u.Email, usernameArg(u.Username), u.PasswordHash,
		string(u.Role), boolToInt(u.Banned), boolToInt(u.IsAdmin), fmtTime(u.CreatedAt))
	if err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.User{}, storage.ErrConflict // e.g. a duplicate username in the tenant
		}
		return storage.User{}, err
	}
	return u, nil
}

func (r userRepo) Get(ctx context.Context, id string) (storage.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, name, email, username, password_hash, role, banned, is_admin, created_at
		   FROM users WHERE tenant_id = ? AND id = ?`), r.tenantID, id))
}

func (r userRepo) ByUsername(ctx context.Context, username string) (storage.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, name, email, username, password_hash, role, banned, is_admin, created_at
		   FROM users WHERE tenant_id = ? AND username = ?`), r.tenantID, username))
}

// ByEmail resolves an owner by email within the tenant (OAuth link-only lookup,
// ADR-0008). An empty email never matches — it would otherwise link any
// credential-less/email-less account. If several users share the email the
// earliest-created wins, so the result is deterministic.
func (r userRepo) ByEmail(ctx context.Context, email string) (storage.User, error) {
	if email == "" {
		return storage.User{}, storage.ErrNotFound
	}
	return r.scanUser(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, name, email, username, password_hash, role, banned, is_admin, created_at
		   FROM users WHERE tenant_id = ? AND email = ?
		  ORDER BY created_at, id LIMIT 1`), r.tenantID, email))
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so one scan body serves
// the single-row lookups and the List iteration.
type rowScanner interface{ Scan(...any) error }

func (r userRepo) scanUser(row *sql.Row) (storage.User, error) {
	u, err := scanUserRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.User{}, storage.ErrNotFound
	}
	return u, err
}

func scanUserRow(row rowScanner) (storage.User, error) {
	var (
		u         storage.User
		username  sql.NullString
		role      string
		banned    int
		isAdmin   int
		createdAt string
	)
	if err := row.Scan(&u.ID, &u.TenantID, &u.Name, &u.Email, &username, &u.PasswordHash,
		&role, &banned, &isAdmin, &createdAt); err != nil {
		return storage.User{}, err
	}
	if username.Valid {
		u.Username = &username.String
	}
	u.Role = storage.UserRole(role)
	u.Banned = banned != 0
	u.IsAdmin = isAdmin != 0
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.User{}, err
	}
	u.CreatedAt = ts
	return u, nil
}

// List returns a page of the tenant's users, oldest-first, with the unpaged total.
func (r userRepo) List(ctx context.Context, p storage.Page) ([]storage.User, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM users WHERE tenant_id = ?`), r.tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT id, tenant_id, name, email, username, password_hash, role, banned, is_admin, created_at
		   FROM users WHERE tenant_id = ?
		  ORDER BY created_at, id LIMIT ? OFFSET ?`), r.tenantID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var users []storage.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// SetRole updates a user's per-tenant role.
func (r userRepo) SetRole(ctx context.Context, userID string, role storage.UserRole) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE users SET role = ? WHERE tenant_id = ? AND id = ?`), string(role), r.tenantID, userID)
	if err != nil {
		return err
	}
	return expectOne(res)
}

// SetBanned sets or clears a user's ban flag.
func (r userRepo) SetBanned(ctx context.Context, userID string, banned bool) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE users SET banned = ? WHERE tenant_id = ? AND id = ?`), boolToInt(banned), r.tenantID, userID)
	if err != nil {
		return err
	}
	return expectOne(res)
}

// SetAdmin sets or clears a user's instance-admin capability (ADR-0010).
func (r userRepo) SetAdmin(ctx context.Context, userID string, isAdmin bool) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE users SET is_admin = ? WHERE tenant_id = ? AND id = ?`), boolToInt(isAdmin), r.tenantID, userID)
	if err != nil {
		return err
	}
	return expectOne(res)
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
