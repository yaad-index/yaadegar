package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type oauthIdentityRepo struct{ baseRepo }

func (r oauthIdentityRepo) Create(ctx context.Context, oi storage.OAuthIdentity) (storage.OAuthIdentity, error) {
	if oi.ID == "" {
		oi.ID = newID()
	}
	if oi.CreatedAt.IsZero() {
		oi.CreatedAt = nowTime()
	}
	oi.TenantID = r.tenantID
	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO oauth_identities (id, tenant_id, user_id, provider, subject, email, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		oi.ID, oi.TenantID, oi.UserID, string(oi.Provider), oi.Subject, oi.Email, fmtTime(oi.CreatedAt))
	if err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.OAuthIdentity{}, storage.ErrConflict
		}
		return storage.OAuthIdentity{}, err
	}
	return oi, nil
}

func (r oauthIdentityRepo) ByProviderSubject(ctx context.Context, provider storage.OAuthProvider, subject string) (storage.OAuthIdentity, error) {
	return r.scan(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, user_id, provider, subject, email, created_at
		   FROM oauth_identities
		  WHERE tenant_id = ? AND provider = ? AND subject = ?`),
		r.tenantID, string(provider), subject))
}

func (r oauthIdentityRepo) ByUserProvider(ctx context.Context, userID string, provider storage.OAuthProvider) (storage.OAuthIdentity, error) {
	return r.scan(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, user_id, provider, subject, email, created_at
		   FROM oauth_identities
		  WHERE tenant_id = ? AND user_id = ? AND provider = ?
		  ORDER BY created_at, id LIMIT 1`),
		r.tenantID, userID, string(provider)))
}

func (r oauthIdentityRepo) scan(row *sql.Row) (storage.OAuthIdentity, error) {
	var (
		oi        storage.OAuthIdentity
		provider  string
		createdAt string
	)
	if err := row.Scan(&oi.ID, &oi.TenantID, &oi.UserID, &provider, &oi.Subject, &oi.Email, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.OAuthIdentity{}, storage.ErrNotFound
		}
		return storage.OAuthIdentity{}, err
	}
	oi.Provider = storage.OAuthProvider(provider)
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.OAuthIdentity{}, err
	}
	oi.CreatedAt = ts
	return oi, nil
}
