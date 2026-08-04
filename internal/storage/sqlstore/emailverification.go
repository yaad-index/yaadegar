package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type emailVerificationRepo struct{ baseRepo }

func (r emailVerificationRepo) Create(ctx context.Context, t storage.EmailVerificationToken) (storage.EmailVerificationToken, error) {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = nowTime()
	}
	t.TenantID = r.tenantID
	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO email_verification_tokens (id, tenant_id, user_id, token_hash, expires_at, used_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		t.ID, t.TenantID, t.UserID, t.TokenHash, fmtTime(t.ExpiresAt), usedAtArg(t.UsedAt), fmtTime(t.CreatedAt))
	if err != nil {
		return storage.EmailVerificationToken{}, err
	}
	return t, nil
}

func (r emailVerificationRepo) ByHash(ctx context.Context, tokenHash string) (storage.EmailVerificationToken, error) {
	return scanEmailVerification(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, user_id, token_hash, expires_at, used_at, created_at
		   FROM email_verification_tokens WHERE tenant_id = ? AND token_hash = ?`), r.tenantID, tokenHash))
}

// LatestByUser returns the newest token for the user (any state), for the
// resend-verification anti-flood guard, which reads its CreatedAt.
func (r emailVerificationRepo) LatestByUser(ctx context.Context, userID string) (storage.EmailVerificationToken, error) {
	return scanEmailVerification(r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, user_id, token_hash, expires_at, used_at, created_at
		   FROM email_verification_tokens WHERE tenant_id = ? AND user_id = ?
		   ORDER BY created_at DESC LIMIT 1`), r.tenantID, userID))
}

// DeleteByUser drops every verification token for the user so a re-minted token
// fully replaces any prior outstanding one.
func (r emailVerificationRepo) DeleteByUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM email_verification_tokens WHERE tenant_id = ? AND user_id = ?`), r.tenantID, userID)
	return err
}

// scanEmailVerification reads one token row (used by ByHash and LatestByUser),
// mapping no-rows to ErrNotFound and parsing the RFC3339Nano string columns in Go.
func scanEmailVerification(row *sql.Row) (storage.EmailVerificationToken, error) {
	var (
		t         storage.EmailVerificationToken
		expiresAt string
		usedAt    sql.NullString
		createdAt string
	)
	if err := row.Scan(&t.ID, &t.TenantID, &t.UserID, &t.TokenHash, &expiresAt, &usedAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.EmailVerificationToken{}, storage.ErrNotFound
		}
		return storage.EmailVerificationToken{}, err
	}
	var err error
	if t.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return storage.EmailVerificationToken{}, err
	}
	if usedAt.Valid {
		used, perr := parseTime(usedAt.String)
		if perr != nil {
			return storage.EmailVerificationToken{}, perr
		}
		t.UsedAt = &used
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return storage.EmailVerificationToken{}, err
	}
	return t, nil
}

// MarkUsed claims the token by setting used_at only while it is still NULL, so a
// concurrent verify or a replay finds nothing to claim. It reports whether this
// call won the claim.
func (r emailVerificationRepo) MarkUsed(ctx context.Context, id string, usedAt time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE email_verification_tokens SET used_at = ?
		   WHERE tenant_id = ? AND id = ? AND used_at IS NULL`), fmtTime(usedAt), r.tenantID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		// Either the id is absent or it was already used. Distinguish so a caller can
		// treat a genuinely missing token as ErrNotFound if it needs to; a losing race
		// on an existing row reports claimed=false with no error.
		var exists int
		if qerr := r.db.QueryRowContext(ctx, r.rb(
			`SELECT COUNT(*) FROM email_verification_tokens WHERE tenant_id = ? AND id = ?`),
			r.tenantID, id).Scan(&exists); qerr != nil {
			return false, qerr
		}
		if exists == 0 {
			return false, storage.ErrNotFound
		}
		return false, nil
	}
	return true, nil
}
