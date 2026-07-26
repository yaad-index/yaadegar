package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type domainRepo struct{ baseRepo }

const domainCols = `id, tenant_id, hostname, cname_target, verified, tls_status, created_at`

func scanDomain(s scanner) (storage.Domain, error) {
	var (
		d         storage.Domain
		verified  int
		createdAt string
	)
	if err := s.Scan(&d.ID, &d.TenantID, &d.Hostname, &d.CNAMETarget, &verified,
		&d.TLSStatus, &createdAt); err != nil {
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
		`INSERT INTO domains (`+domainCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.TenantID, d.Hostname, d.CNAMETarget, boolToInt(d.Verified),
		d.TLSStatus, fmtTime(d.CreatedAt))
	if err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.Domain{}, storage.ErrConflict
		}
		return storage.Domain{}, err
	}
	return d, nil
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
