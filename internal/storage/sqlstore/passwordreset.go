package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type passwordResetRepo struct{ baseRepo }

func (r passwordResetRepo) Create(ctx context.Context, t storage.PasswordResetToken) (storage.PasswordResetToken, error) {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = nowTime()
	}
	t.TenantID = r.tenantID
	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO password_reset_tokens (id, tenant_id, user_id, token_hash, expires_at, used_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		t.ID, t.TenantID, t.UserID, t.TokenHash, fmtTime(t.ExpiresAt), usedAtArg(t.UsedAt), fmtTime(t.CreatedAt))
	if err != nil {
		return storage.PasswordResetToken{}, err
	}
	return t, nil
}

func (r passwordResetRepo) ByHash(ctx context.Context, tokenHash string) (storage.PasswordResetToken, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT id, tenant_id, user_id, token_hash, expires_at, used_at, created_at
		   FROM password_reset_tokens WHERE tenant_id = ? AND token_hash = ?`), r.tenantID, tokenHash)
	var (
		t         storage.PasswordResetToken
		expiresAt string
		usedAt    sql.NullString
		createdAt string
	)
	if err := row.Scan(&t.ID, &t.TenantID, &t.UserID, &t.TokenHash, &expiresAt, &usedAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.PasswordResetToken{}, storage.ErrNotFound
		}
		return storage.PasswordResetToken{}, err
	}
	var err error
	if t.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return storage.PasswordResetToken{}, err
	}
	if usedAt.Valid {
		used, perr := parseTime(usedAt.String)
		if perr != nil {
			return storage.PasswordResetToken{}, perr
		}
		t.UsedAt = &used
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return storage.PasswordResetToken{}, err
	}
	return t, nil
}

// MarkUsed claims the token by setting used_at only while it is still NULL, so a
// concurrent confirm or a replay finds nothing to claim. It reports whether this
// call won the claim.
func (r passwordResetRepo) MarkUsed(ctx context.Context, id string, usedAt time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE password_reset_tokens SET used_at = ?
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
			`SELECT COUNT(*) FROM password_reset_tokens WHERE tenant_id = ? AND id = ?`),
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

// errTokenAlreadyClaimed is an internal sentinel used to abort (and roll back) the
// ConfirmReset transaction when the token was already consumed. It never leaves the
// package — ConfirmReset maps it to claimed=false with no error.
var errTokenAlreadyClaimed = errors.New("sqlstore: reset token already claimed")

// ConfirmReset performs the confirm mutation set in one transaction (#166), reusing
// the shared row-lock boundary. It locks the user, sets the password (bumping
// credential_version), activates a still-pending account, and finally claims the
// token — the token claim is the commit gate: if a concurrent confirm already used
// it, this affects 0 rows and the whole transaction rolls back, so the password and
// activation writes never persist without the token being consumed (and vice versa).
func (r passwordResetRepo) ConfirmReset(ctx context.Context, tokenID, userID, passwordHash string, usedAt time.Time) (bool, error) {
	claimed := false
	err := r.withRowLock(ctx, "users", userID, func(tx *sql.Tx) error {
		// Establish the password — bumps credential_version, invalidating every session.
		if _, err := tx.ExecContext(ctx, r.rb(
			`UPDATE users SET password_hash = ?, credential_version = credential_version + 1
			   WHERE tenant_id = ? AND id = ?`), passwordHash, r.tenantID, userID); err != nil {
			return err
		}
		// Activate a still-pending account (proving email ownership completes activation,
		// ADR-0012 cut 1b). A no-op for an already-active account.
		if _, err := tx.ExecContext(ctx, r.rb(
			`UPDATE users SET status = ? WHERE tenant_id = ? AND id = ? AND status = ?`),
			storage.UserStatusActive, r.tenantID, userID, storage.UserStatusPending); err != nil {
			return err
		}
		// Commit gate: claim the token only while still unused. 0 rows → a concurrent
		// confirm won (or the token is gone) → abort → the writes above roll back.
		res, err := tx.ExecContext(ctx, r.rb(
			`UPDATE password_reset_tokens SET used_at = ?
			   WHERE tenant_id = ? AND id = ? AND used_at IS NULL`), fmtTime(usedAt), r.tenantID, tokenID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return errTokenAlreadyClaimed
		}
		claimed = true
		return nil
	})
	if errors.Is(err, errTokenAlreadyClaimed) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return claimed, nil
}

// usedAtArg maps an optional used-at timestamp to a NULL-able driver value.
func usedAtArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return fmtTime(*t)
}
