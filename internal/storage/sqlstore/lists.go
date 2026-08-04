package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type listRepo struct{ baseRepo }

// listCols are the physical list columns, used for INSERT. Ownership is no longer
// a column here — it lives in list_owners (ADR-0005 §7).
const listCols = `id, tenant_id, title, visibility, share_slug,
	event_date, decay_days, active, created_at, reserver_tier, reserver_confirm_window`

// listSelectCols is listCols plus allow_cobuy (the list co-buy default, #100 — read
// only; Create relies on its NOT NULL DEFAULT 1) and two correlated subqueries used
// for reads: the derived item_count, and the (v1 sole) owner id from list_owners.
const listSelectCols = listCols + `, allow_cobuy, thank_you_template, description,
	(SELECT COUNT(*) FROM items
	  WHERE items.tenant_id = lists.tenant_id AND items.list_id = lists.id) AS item_count,
	(SELECT user_id FROM list_owners
	  WHERE list_owners.list_id = lists.id
	  ORDER BY added_at, user_id LIMIT 1) AS owner_id`

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// decayInherit is the storage-internal encoding of "inherit the instance default"
// for the lists.decay_days column (which is NOT NULL). It never escapes the
// scan/insert path: the domain type exposes *int (nil = inherit) instead.
const decayInherit = -1

// decayDaysFromStorage maps the raw column value to the domain override pointer.
func decayDaysFromStorage(v int) *int {
	if v == decayInherit {
		return nil
	}
	d := v
	return &d
}

// decayDaysToStorage maps the domain override pointer to the raw column value.
func decayDaysToStorage(p *int) int {
	if p == nil {
		return decayInherit
	}
	return *p
}

// reserver_confirm_window (integer minutes) mirrors decay_days' encoding exactly:
// a NOT NULL column where the decayInherit (-1) sentinel means "inherit the
// instance default", exposed to the domain as *int (nil = inherit).
func confirmWindowFromStorage(v int) *int {
	if v == decayInherit {
		return nil
	}
	m := v
	return &m
}

func confirmWindowToStorage(p *int) int {
	if p == nil {
		return decayInherit
	}
	return *p
}

// reserver_tier is a nullable TEXT column: NULL means "inherit the instance
// default", so the domain override is a plain pointer (nil = inherit).
func reserverTierFromStorage(v sql.NullString) *storage.ReserverTier {
	if !v.Valid || v.String == "" {
		return nil
	}
	t := storage.ReserverTier(v.String)
	return &t
}

func reserverTierToStorage(p *storage.ReserverTier) any {
	if p == nil {
		return nil // NULL = inherit
	}
	return string(*p)
}

// items.allow_cobuy is a nullable INTEGER: NULL means "inherit the list default",
// so the item override is a plain pointer (nil = inherit), 0/1 an explicit off/on.
func allowCobuyFromStorage(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 != 0
	return &b
}

func allowCobuyToStorage(p *bool) any {
	if p == nil {
		return nil // NULL = inherit the list default
	}
	return boolToInt(*p)
}

func scanList(s scanner) (storage.List, error) {
	var (
		l             storage.List
		eventDate     sql.NullString
		decayDays     int
		active        int
		createdAt     string
		reserverTier  sql.NullString
		confirmWindow int
		allowCobuy    int
		thankYou      string
		description   string
		ownerID       sql.NullString
	)
	if err := s.Scan(&l.ID, &l.TenantID, &l.Title, &l.Visibility,
		&l.ShareSlug, &eventDate, &decayDays, &active, &createdAt, &reserverTier,
		&confirmWindow, &allowCobuy, &thankYou, &description, &l.ItemCount, &ownerID); err != nil {
		return storage.List{}, err
	}
	l.OwnerID = ownerID.String // derived: the v1 sole owner (empty only if orphaned)
	l.DecayDays = decayDaysFromStorage(decayDays)
	l.ReserverTier = reserverTierFromStorage(reserverTier)
	l.ReserverConfirmWindowMinutes = confirmWindowFromStorage(confirmWindow)
	l.AllowCobuy = allowCobuy != 0
	l.ThankYouTemplate = thankYou
	l.Description = description
	ed, err := datePtr(eventDate)
	if err != nil {
		return storage.List{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.List{}, err
	}
	l.EventDate = ed
	l.Active = active != 0
	l.CreatedAt = ts
	return l, nil
}

// Create inserts the list and records ownerID as its sole owner, atomically.
func (r listRepo) Create(ctx context.Context, l storage.List, ownerID string) (storage.List, error) {
	if l.ID == "" {
		l.ID = newID()
	}
	if l.ShareSlug == "" {
		slug, err := newSlug()
		if err != nil {
			return storage.List{}, err
		}
		l.ShareSlug = slug
	}
	if l.Visibility == "" {
		l.Visibility = storage.VisibilityPrivate
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = nowTime()
	}
	l.TenantID = r.tenantID
	l.OwnerID = ownerID

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.List{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.rb(
		`INSERT INTO lists (`+listCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		l.ID, l.TenantID, l.Title, l.Visibility, l.ShareSlug,
		nullDate(l.EventDate), decayDaysToStorage(l.DecayDays), boolToInt(l.Active),
		fmtTime(l.CreatedAt), reserverTierToStorage(l.ReserverTier),
		confirmWindowToStorage(l.ReserverConfirmWindowMinutes)); err != nil {
		if r.d.isUniqueViolation(err) {
			return storage.List{}, storage.ErrConflict
		}
		return storage.List{}, err
	}
	if _, err := tx.ExecContext(ctx, r.rb(
		`INSERT INTO list_owners (list_id, user_id, added_at) VALUES (?, ?, ?)`),
		l.ID, ownerID, fmtTime(l.CreatedAt)); err != nil {
		return storage.List{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.List{}, err
	}
	// allow_cobuy is omitted from the INSERT so its NOT NULL DEFAULT 1 applies — a
	// new list is always co-buy-enabled; the owner opts out later via Update. Mirror
	// that default onto the returned struct (Create does not re-read).
	l.AllowCobuy = true
	return l, nil
}

func (r listRepo) get(ctx context.Context, cond string, arg string) (storage.List, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+listSelectCols+` FROM lists WHERE tenant_id = ? AND `+cond), r.tenantID, arg)
	l, err := scanList(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.List{}, storage.ErrNotFound
		}
		return storage.List{}, err
	}
	return l, nil
}

func (r listRepo) Get(ctx context.Context, id string) (storage.List, error) {
	return r.get(ctx, `id = ?`, id)
}

func (r listRepo) GetBySlug(ctx context.Context, shareSlug string) (storage.List, error) {
	return r.get(ctx, `share_slug = ?`, shareSlug)
}

// List returns the lists ownerID owns, joined through list_owners.
func (r listRepo) List(ctx context.Context, ownerID string, p storage.Page) ([]storage.List, int, error) {
	const ownerFilter = ` AND EXISTS (SELECT 1 FROM list_owners lo
		WHERE lo.list_id = lists.id AND lo.user_id = ?)`

	var total int
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM lists WHERE tenant_id = ?`+ownerFilter),
		r.tenantID, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+listSelectCols+` FROM lists
		  WHERE tenant_id = ?`+ownerFilter+`
		  ORDER BY created_at DESC, id
		  LIMIT ? OFFSET ?`),
		r.tenantID, ownerID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.List
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

func (r listRepo) Update(ctx context.Context, l storage.List) (storage.List, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE lists SET title = ?, visibility = ?, event_date = ?,
		        decay_days = ?, active = ?, reserver_tier = ?, reserver_confirm_window = ?,
		        allow_cobuy = ?, thank_you_template = ?, description = ?
		  WHERE tenant_id = ? AND id = ?`),
		l.Title, l.Visibility, nullDate(l.EventDate), decayDaysToStorage(l.DecayDays),
		boolToInt(l.Active), reserverTierToStorage(l.ReserverTier),
		confirmWindowToStorage(l.ReserverConfirmWindowMinutes), boolToInt(l.AllowCobuy),
		l.ThankYouTemplate, l.Description, r.tenantID, l.ID)
	if err != nil {
		return storage.List{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.List{}, err
	}
	return r.Get(ctx, l.ID)
}

// Delete removes the list and its ownership rows. The list_owners delete is scoped
// through this tenant's lists so a foreign list id can never touch another tenant's
// rows; it also backs the FK cascade for the SQLite dialect (which runs without
// foreign-key enforcement).
func (r listRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.rb(
		`DELETE FROM list_owners
		  WHERE list_id IN (SELECT id FROM lists WHERE tenant_id = ? AND id = ?)`),
		r.tenantID, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, r.rb(
		`DELETE FROM lists WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	if err != nil {
		return err
	}
	if err := expectOne(res); err != nil {
		return err
	}
	return tx.Commit()
}

// IsOwner reports whether userID owns listID, scoped to the bound tenant.
func (r listRepo) IsOwner(ctx context.Context, listID, userID string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM list_owners lo
		   JOIN lists l ON l.id = lo.list_id
		  WHERE l.tenant_id = ? AND lo.list_id = ? AND lo.user_id = ?`),
		r.tenantID, listID, userID).Scan(&n)
	return n > 0, err
}

// Owners returns the user ids owning listID (tenant-scoped).
func (r listRepo) Owners(ctx context.Context, listID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT lo.user_id FROM list_owners lo
		   JOIN lists l ON l.id = lo.list_id
		  WHERE l.tenant_id = ? AND lo.list_id = ?
		  ORDER BY lo.added_at, lo.user_id`),
		r.tenantID, listID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddOwner records userID as an owner of listID, enforcing the v1 single-owner
// rule: if the list already has a different owner it returns ErrConflict. Adding
// the existing sole owner again is a no-op.
func (r listRepo) AddOwner(ctx context.Context, listID, userID string) error {
	owners, err := r.Owners(ctx, listID)
	if err != nil {
		return err
	}
	for _, o := range owners {
		if o == userID {
			return nil // already the owner
		}
	}
	if len(owners) > 0 {
		return storage.ErrConflict // v1: exactly one owner (co-ownership is #25)
	}
	// Tenant-scope the insert (mirrors RemoveOwner): the row is created only when the
	// list belongs to the bound tenant, so a foreign-tenant listID inserts nothing —
	// closing a latent cross-tenant ownership hole before #25 wires AddOwner to a
	// handler. The read-then-insert single-owner race is deferred to #25 (which relaxes
	// the single-owner rule anyway).
	_, err = r.db.ExecContext(ctx, r.rb(
		`INSERT INTO list_owners (list_id, user_id, added_at)
		 SELECT id, ?, ? FROM lists WHERE tenant_id = ? AND id = ?`),
		userID, fmtTime(nowTime()), r.tenantID, listID)
	return err
}

// RemoveOwner removes userID as an owner of listID (tenant-scoped; no-op if absent).
func (r listRepo) RemoveOwner(ctx context.Context, listID, userID string) error {
	_, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM list_owners
		  WHERE user_id = ? AND list_id IN (SELECT id FROM lists WHERE tenant_id = ? AND id = ?)`),
		userID, r.tenantID, listID)
	return err
}
