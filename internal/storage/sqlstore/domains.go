package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type domainRepo struct{ baseRepo }

const domainCols = `id, tenant_id, hostname, cname_target, verified, tls_status,
	verification_token, created_at`

func scanDomain(s scanner) (storage.Domain, error) {
	var (
		d         storage.Domain
		verified  int
		createdAt string
	)
	if err := s.Scan(&d.ID, &d.TenantID, &d.Hostname, &d.CNAMETarget, &verified,
		&d.TLSStatus, &d.VerificationToken, &createdAt); err != nil {
		return storage.Domain{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Domain{}, err
	}
	d.Verified = verified != 0
	d.CreatedAt = ts
	return d, nil
}

func (r domainRepo) Create(ctx context.Context, d storage.Domain) (storage.Domain, error) {
	if d.ID == "" {
		d.ID = newID()
	}
	if d.TLSStatus == "" {
		d.TLSStatus = storage.TLSNone
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = nowTime()
	}
	d.TenantID = r.tenantID

	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO domains (`+domainCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.TenantID, d.Hostname, d.CNAMETarget, boolToInt(d.Verified),
		d.TLSStatus, d.VerificationToken, fmtTime(d.CreatedAt))
	if err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.Domain{}, storage.ErrConflict
		}
		return storage.Domain{}, err
	}
	return d, nil
}

// CreateReclaimingExpired inserts the domain, first reclaiming an existing claim on
// the same hostname when that claim is unverified and older than expiredBefore.
// This frees a hostname parked by an unverified squatter (the add-time namespace
// DoS in ADR-0004 §4) without ever disturbing a verified domain. The lookup +
// reclaim + insert run in one transaction under a row lock on the existing claim,
// so two concurrent adds cannot both reclaim. The lookup is intentionally global
// (not tenant-scoped): a squatting claim may belong to another tenant. A zero
// expiredBefore disables reclaiming.
func (r domainRepo) CreateReclaimingExpired(ctx context.Context, dom storage.Domain, expiredBefore time.Time) (storage.Domain, error) {
	if dom.ID == "" {
		dom.ID = newID()
	}
	if dom.TLSStatus == "" {
		dom.TLSStatus = storage.TLSNone
	}
	if dom.CreatedAt.IsZero() {
		dom.CreatedAt = nowTime()
	}
	dom.TenantID = r.tenantID

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Domain{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock any existing claim on this hostname (global — a squatter may be another
	// tenant). Reclaim it only when it is unverified and past the expiry window.
	var (
		existingID string
		verified   int
		createdAt  string
	)
	err = tx.QueryRowContext(ctx, r.rb(
		`SELECT id, verified, created_at FROM domains WHERE hostname = ?`+r.d.forUpdate()),
		dom.Hostname).Scan(&existingID, &verified, &createdAt)
	switch {
	case err == nil:
		if verified != 0 {
			return storage.Domain{}, storage.ErrConflict // verified never expires
		}
		ca, perr := parseTime(createdAt)
		if perr != nil {
			return storage.Domain{}, perr
		}
		if !ca.Before(expiredBefore) {
			return storage.Domain{}, storage.ErrConflict // still within the window
		}
		if _, derr := tx.ExecContext(ctx, r.rb(
			`DELETE FROM domains WHERE id = ?`), existingID); derr != nil {
			return storage.Domain{}, derr
		}
	case errors.Is(err, sql.ErrNoRows):
		// No existing claim — a normal first registration.
	default:
		return storage.Domain{}, err
	}

	if _, err := tx.ExecContext(ctx, r.rb(
		`INSERT INTO domains (`+domainCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		dom.ID, dom.TenantID, dom.Hostname, dom.CNAMETarget, boolToInt(dom.Verified),
		dom.TLSStatus, dom.VerificationToken, fmtTime(dom.CreatedAt)); err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.Domain{}, storage.ErrConflict
		}
		return storage.Domain{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.Domain{}, err
	}
	return dom, nil
}

func (r domainRepo) Get(ctx context.Context, id string) (storage.Domain, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+domainCols+` FROM domains WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	d, err := scanDomain(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Domain{}, storage.ErrNotFound
		}
		return storage.Domain{}, err
	}
	return d, nil
}

func (r domainRepo) List(ctx context.Context) ([]storage.Domain, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+domainCols+` FROM domains WHERE tenant_id = ?
		  ORDER BY created_at, id`), r.tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r domainRepo) Update(ctx context.Context, d storage.Domain) (storage.Domain, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE domains SET cname_target = ?, verified = ?, tls_status = ?
		  WHERE tenant_id = ? AND id = ?`),
		d.CNAMETarget, boolToInt(d.Verified), d.TLSStatus, r.tenantID, d.ID)
	if err != nil {
		return storage.Domain{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.Domain{}, err
	}
	return r.Get(ctx, d.ID)
}

func (r domainRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM domains WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}
