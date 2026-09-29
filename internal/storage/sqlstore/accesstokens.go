package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type accessTokenRepo struct{ baseRepo }

const accessTokenCols = `id, tenant_id, user_id, name, token_hash, last4, created_at, expires_at, last_used_at, revoked_at`

// accessTokenScanner is satisfied by *sql.Row and *sql.Rows.
type accessTokenScanner interface {
	Scan(dest ...any) error
}

func scanAccessToken(s accessTokenScanner) (storage.AccessToken, error) {
	var (
		t                              storage.AccessToken
		createdAt                      string
		expiresAt, lastUsed, revokedAt sql.NullString
	)
	if err := s.Scan(&t.ID, &t.TenantID, &t.UserID, &t.Name, &t.TokenHash, &t.Last4,
		&createdAt, &expiresAt, &lastUsed, &revokedAt); err != nil {
		return storage.AccessToken{}, err
	}
	var err error
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return storage.AccessToken{}, err
	}
	if t.ExpiresAt, err = timePtr(expiresAt); err != nil {
		return storage.AccessToken{}, err
	}
	if t.LastUsedAt, err = timePtr(lastUsed); err != nil {
		return storage.AccessToken{}, err
	}
	if t.RevokedAt, err = timePtr(revokedAt); err != nil {
		return storage.AccessToken{}, err
	}
	return t, nil
}

// Create inserts the token under the user's row lock, after counting the user's
// active tokens inside the same transaction, so two concurrent creations cannot
// both pass the limit.
func (r accessTokenRepo) Create(ctx context.Context, t storage.AccessToken, maxActive int, now time.Time) (storage.AccessToken, error) {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = r.now()
	}
	t.TenantID = r.tenantID
	err := r.withRowLock(ctx, "users", t.UserID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, r.rb(
			`SELECT `+accessTokenCols+` FROM access_tokens WHERE tenant_id = ? AND user_id = ?`),
			r.tenantID, t.UserID)
		if err != nil {
			return err
		}
		active := 0
		for rows.Next() {
			existing, err := scanAccessToken(rows)
			if err != nil {
				_ = rows.Close()
				return err
			}
			if existing.ActiveAt(now) {
				active++
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if active >= maxActive {
			return storage.ErrTooManyTokens
		}
		_, err = tx.ExecContext(ctx, r.rb(
			`INSERT INTO access_tokens (`+accessTokenCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			t.ID, t.TenantID, t.UserID, t.Name, t.TokenHash, t.Last4, fmtTime(t.CreatedAt),
			nullTime(t.ExpiresAt), nullTime(t.LastUsedAt), nullTime(t.RevokedAt))
		return err
	})
	if err != nil {
		return storage.AccessToken{}, err
	}
	return t, nil
}

func (r accessTokenRepo) ByHash(ctx context.Context, tokenHash string) (storage.AccessToken, error) {
	t, err := scanAccessToken(r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+accessTokenCols+` FROM access_tokens WHERE tenant_id = ? AND token_hash = ?`),
		r.tenantID, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.AccessToken{}, storage.ErrNotFound
	}
	return t, err
}

func (r accessTokenRepo) ListByUser(ctx context.Context, userID string) ([]storage.AccessToken, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+accessTokenCols+` FROM access_tokens WHERE tenant_id = ? AND user_id = ?
		  ORDER BY created_at DESC, id DESC`), r.tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.AccessToken
	for rows.Next() {
		t, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r accessTokenRepo) Revoke(ctx context.Context, userID, id string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE access_tokens SET revoked_at = ?
		  WHERE tenant_id = ? AND user_id = ? AND id = ? AND revoked_at IS NULL`),
		fmtTime(at), r.tenantID, userID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	// Either the user holds no such token, or it was already revoked.
	var exists int
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM access_tokens WHERE tenant_id = ? AND user_id = ? AND id = ?`),
		r.tenantID, userID, id).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, storage.ErrNotFound
	}
	return false, nil
}

func (r accessTokenRepo) RevokeAllForUser(ctx context.Context, userID string, at time.Time) (int64, error) {
	return revokeAllAccessTokens(ctx, r.db, r.baseRepo, userID, at)
}

// revokeAllAccessTokens revokes every unrevoked token of the user through the given
// executor, so a caller can run it inside a wider transaction (the password-reset
// confirm revokes tokens in the same transaction that resets the password).
func revokeAllAccessTokens(ctx context.Context, ex execer, b baseRepo, userID string, at time.Time) (int64, error) {
	res, err := ex.ExecContext(ctx, b.rb(
		`UPDATE access_tokens SET revoked_at = ?
		  WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL`),
		fmtTime(at), b.tenantID, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TouchLastUsed reads the stored last-used time and writes the new one only when
// it is unset or at least minInterval older. The comparison is done in Go; the
// conditional UPDATE matches the value just read, so of two concurrent touches at
// most one writes.
func (r accessTokenRepo) TouchLastUsed(ctx context.Context, id string, at time.Time, minInterval time.Duration) (bool, error) {
	var prev sql.NullString
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT last_used_at FROM access_tokens WHERE tenant_id = ? AND id = ?`),
		r.tenantID, id).Scan(&prev); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, storage.ErrNotFound
		}
		return false, err
	}
	last, err := timePtr(prev)
	if err != nil {
		return false, err
	}
	if last != nil && at.Sub(*last) < minInterval {
		return false, nil
	}
	query := `UPDATE access_tokens SET last_used_at = ? WHERE tenant_id = ? AND id = ? AND last_used_at IS NULL`
	args := []any{fmtTime(at), r.tenantID, id}
	if prev.Valid {
		query = `UPDATE access_tokens SET last_used_at = ? WHERE tenant_id = ? AND id = ? AND last_used_at = ?`
		args = append(args, prev.String)
	}
	res, err := r.db.ExecContext(ctx, r.rb(query), args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
