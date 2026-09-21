package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type itemRepo struct{ baseRepo }

const itemCols = `id, tenant_id, list_id, name, url, image_url,
	price_amount_minor, price_currency, note, priority, quantity_wanted, allow_cobuy,
	thank_you_template, created_at, archived_at`

func scanItem(s scanner) (storage.Item, error) {
	var (
		it         storage.Item
		url        sql.NullString
		imageURL   sql.NullString
		amount     sql.NullInt64
		currency   sql.NullString
		note       sql.NullString
		allowCobuy sql.NullInt64
		thankYou   sql.NullString
		createdAt  string
		archivedAt sql.NullString
	)
	if err := s.Scan(&it.ID, &it.TenantID, &it.ListID, &it.Name, &url, &imageURL,
		&amount, &currency, &note, &it.Priority, &it.QuantityWanted, &allowCobuy,
		&thankYou, &createdAt, &archivedAt); err != nil {
		return storage.Item{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Item{}, err
	}
	archived, err := timePtr(archivedAt)
	if err != nil {
		return storage.Item{}, err
	}
	it.URL = strPtr(url)
	it.ImageURL = strPtr(imageURL)
	it.Price = pricePtr(amount, currency)
	it.Note = strPtr(note)
	it.AllowCobuy = allowCobuyFromStorage(allowCobuy)
	it.ThankYouTemplate = strPtr(thankYou)
	it.CreatedAt = ts
	it.ArchivedAt = archived
	return it, nil
}

// prep fills server-set defaults and binds the tenant.
func (r itemRepo) prep(it storage.Item) storage.Item {
	if it.ID == "" {
		it.ID = newID()
	}
	if it.QuantityWanted < 1 {
		it.QuantityWanted = 1
	}
	if it.CreatedAt.IsZero() {
		it.CreatedAt = nowTime()
	}
	it.TenantID = r.tenantID
	return it
}

// insert writes a prepared item via x (a *sql.DB or *sql.Tx).
func (r itemRepo) insert(ctx context.Context, x execer, it storage.Item) error {
	amount, currency := priceCols(it.Price)
	_, err := x.ExecContext(ctx, r.rb(
		`INSERT INTO items (`+itemCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		it.ID, it.TenantID, it.ListID, it.Name, nullStr(it.URL), nullStr(it.ImageURL),
		amount, currency, nullStr(it.Note), it.Priority, it.QuantityWanted,
		allowCobuyToStorage(it.AllowCobuy), nullStr(it.ThankYouTemplate), fmtTime(it.CreatedAt),
		nullTime(it.ArchivedAt))
	return err
}

func (r itemRepo) Create(ctx context.Context, it storage.Item) (storage.Item, error) {
	it = r.prep(it)
	if err := r.insert(ctx, r.db, it); err != nil {
		return storage.Item{}, err
	}
	return it, nil
}

// CreateMany inserts all items in a single transaction (#26 import): either every
// item is created or none is, so a mid-batch DB failure never leaves a
// half-imported list. Each item is tenant-bound and defaulted like Create.
func (r itemRepo) CreateMany(ctx context.Context, items []storage.Item) ([]storage.Item, error) {
	if len(items) == 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	out := make([]storage.Item, 0, len(items))
	for _, it := range items {
		it = r.prep(it)
		if err := r.insert(ctx, tx, it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r itemRepo) Get(ctx context.Context, id string) (storage.Item, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT `+itemCols+` FROM items WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	it, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Item{}, storage.ErrNotFound
		}
		return storage.Item{}, err
	}
	return it, nil
}

// archivedPredicate is the SQL fragment that narrows a read to live items, or
// nothing at all when archived ones are wanted too. It is written once and shared
// by the count and the page below, because a count taken over a different set
// from the page it describes is a paging bug that only shows up once somebody
// archives something.
func archivedPredicate(f storage.ArchivedFilter) string {
	if f == storage.IncludeArchived {
		return ""
	}
	return " AND archived_at IS NULL"
}

func (r itemRepo) ListByList(ctx context.Context, listID string, p storage.Page, archived storage.ArchivedFilter) ([]storage.Item, int, error) {
	pred := archivedPredicate(archived)

	var total int
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM items WHERE tenant_id = ? AND list_id = ?`+pred),
		r.tenantID, listID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+itemCols+` FROM items
		  WHERE tenant_id = ? AND list_id = ?`+pred+`
		  ORDER BY priority DESC, created_at, id
		  LIMIT ? OFFSET ?`),
		r.tenantID, listID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var out []storage.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (r itemRepo) Update(ctx context.Context, it storage.Item) (storage.Item, error) {
	// Keep the wanted quantity at least 1, consistent with Create — a zero would
	// make the item permanently unreservable (reserved 0 + any qty always exceeds
	// the wanted 0).
	if it.QuantityWanted < 1 {
		it.QuantityWanted = 1
	}
	amount, currency := priceCols(it.Price)
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE items SET name = ?, url = ?, image_url = ?, price_amount_minor = ?,
		        price_currency = ?, note = ?, priority = ?, quantity_wanted = ?, allow_cobuy = ?,
		        thank_you_template = ?
		  WHERE tenant_id = ? AND id = ?`),
		it.Name, nullStr(it.URL), nullStr(it.ImageURL), amount, currency,
		nullStr(it.Note), it.Priority, it.QuantityWanted, allowCobuyToStorage(it.AllowCobuy),
		nullStr(it.ThankYouTemplate), r.tenantID, it.ID)
	if err != nil {
		return storage.Item{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.Item{}, err
	}
	return r.Get(ctx, it.ID)
}

// Archive stamps archived_at, reporting whether this call is the one that did it.
//
// The guard is `archived_at IS NULL` rather than a read-then-write, so two
// concurrent archives produce exactly one true: the caller uses that to send the
// giver notification once. Re-archiving an already-archived item is not an error
// — it is simply not news — and the stamp is left at the original moment rather
// than being refreshed, because it records when the owner finished with the item.
func (r itemRepo) Archive(ctx context.Context, id string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.rb(
		`UPDATE items SET archived_at = ?
		  WHERE tenant_id = ? AND id = ? AND archived_at IS NULL`),
		fmtTime(at), r.tenantID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		// Either the item is gone or it was already archived — distinguish, so a
		// caller can return 404 for the first and a no-op success for the second.
		if _, err := r.Get(ctx, id); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// Unarchive clears the stamp, reporting whether this call is the one that did it.
// The item becomes reservable again and its reservations resume decaying — the
// archive is undone exactly, with no residue, which is what makes it safe for an
// owner to use on an item they are not sure about.
func (r itemRepo) Unarchive(ctx context.Context, id string, at time.Time) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, r.rb(
		`UPDATE items SET archived_at = NULL
		  WHERE tenant_id = ? AND id = ? AND archived_at IS NOT NULL`),
		r.tenantID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		if _, err := r.Get(ctx, id); err != nil {
			return false, err
		}
		return false, nil
	}

	// Restart the decay clocks the archive suspended. This reaches into
	// reservations from the item repo on purpose, and in the same transaction,
	// because the two halves are one operation: an item whose archive flag is
	// cleared while its reservations still carry pre-archive timestamps is the
	// broken state, not a step toward the fixed one.
	//
	// ⚠️ The archive suspends CANDIDACY, not the clock. Sweeper.step compares
	// absolute wall time — now.Sub(last_activity_at) for active, now.Sub(state_at)
	// for pending_confirmation and reserver_notified — so an item archived for
	// longer than the window comes back already past it, and the giver is chased
	// or expired on the very next sweep for time that passed while the item was
	// off the list and they could do nothing about it.
	//
	// RESTART rather than resume, and both columns rather than only the one that
	// drives `active`. Restart, because the alternative — shifting the stamps
	// forward by the archived duration to preserve elapsed-time-within-state —
	// hands a reserver_notified giver whatever slice of the response window was
	// left, which after a long archive can be minutes. Erring toward keeping the
	// hold is the safe direction here: an item that stays reserved slightly too
	// long is recoverable, an item wrongly freed is the double-buy this whole
	// issue is about. Both columns, because state_at drives two of the three
	// states and fixing only last_activity_at would leave a notified reservation
	// silently expiring on the next sweep — the worst of the three, since that
	// path sends no email at all.
	//
	// Expired reservations are left alone: they are terminal and restarting their
	// clock would say something false about a hold that no longer exists.
	if _, err := tx.ExecContext(ctx, r.rb(
		`UPDATE reservations SET last_activity_at = ?, state_at = ?
		  WHERE tenant_id = ? AND item_id = ? AND state != ?`),
		fmtTime(at), fmtTime(at), r.tenantID, id, string(storage.StateExpired)); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r itemRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.rb(
		`DELETE FROM items WHERE tenant_id = ? AND id = ?`), r.tenantID, id)
	if err != nil {
		return err
	}
	return expectOne(res)
}

// ReservedQuantity sums reservation quantities on an item (ADR-0003: aggregate
// helper so #5 derives availability without N+1 reads).
func (r itemRepo) ReservedQuantity(ctx context.Context, itemID string) (int, error) {
	var n sql.NullInt64
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COALESCE(SUM(quantity), 0) FROM reservations
		  WHERE tenant_id = ? AND item_id = ? AND state != 'expired'`),
		r.tenantID, itemID).Scan(&n); err != nil {
		return 0, err
	}
	return int(n.Int64), nil
}

// FundedAmount sums non-terminal contribution pledges on an item. It returns the
// summed minor units under the currency of the pledges; a currency-less zero is
// returned when there are none. Mixed-currency pledges are out of scope for v1
// (co-buying is single-currency per item).
func (r itemRepo) FundedAmount(ctx context.Context, itemID string) (storage.Money, error) {
	row := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COALESCE(SUM(pledged_amount_minor), 0), MAX(pledged_currency)
		   FROM contributions
		  WHERE tenant_id = ? AND item_id = ? AND status IN (?, ?, ?)`),
		r.tenantID, itemID,
		string(storage.ContributionPending),
		string(storage.ContributionMatched),
		string(storage.ContributionConfirmed))

	var (
		amount   int64
		currency sql.NullString
	)
	if err := row.Scan(&amount, &currency); err != nil {
		return storage.Money{}, err
	}
	return storage.Money{AmountMinor: amount, Currency: currency.String}, nil
}

// ReservedQuantitiesByList returns reserved quantity per item across a list in
// one grouped query (batch form of ReservedQuantity — avoids N+1).
func (r itemRepo) ReservedQuantitiesByList(ctx context.Context, listID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT res.item_id, COALESCE(SUM(res.quantity), 0)
		   FROM reservations res
		   JOIN items it ON it.tenant_id = res.tenant_id AND it.id = res.item_id
		  WHERE res.tenant_id = ? AND it.list_id = ? AND res.state != 'expired'
		  GROUP BY res.item_id`), r.tenantID, listID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := map[string]int{}
	for rows.Next() {
		var (
			itemID string
			qty    int
		)
		if err := rows.Scan(&itemID, &qty); err != nil {
			return nil, err
		}
		out[itemID] = qty
	}
	return out, rows.Err()
}

// FundedAmountsByList returns funded amount per item across a list in one grouped
// query (batch form of FundedAmount).
func (r itemRepo) FundedAmountsByList(ctx context.Context, listID string) (map[string]storage.Money, error) {
	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT con.item_id, COALESCE(SUM(con.pledged_amount_minor), 0), MAX(con.pledged_currency)
		   FROM contributions con
		   JOIN items it ON it.tenant_id = con.tenant_id AND it.id = con.item_id
		  WHERE con.tenant_id = ? AND it.list_id = ? AND con.status IN (?, ?, ?)
		  GROUP BY con.item_id`),
		r.tenantID, listID,
		string(storage.ContributionPending),
		string(storage.ContributionMatched),
		string(storage.ContributionConfirmed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := map[string]storage.Money{}
	for rows.Next() {
		var (
			itemID   string
			amount   int64
			currency sql.NullString
		)
		if err := rows.Scan(&itemID, &amount, &currency); err != nil {
			return nil, err
		}
		out[itemID] = storage.Money{AmountMinor: amount, Currency: currency.String}
	}
	return out, rows.Err()
}

// PreviewsByLists returns up to perList item previews per list in one query, the
// batch feed for the dashboard preview cluster (#207). A window function ranks each
// list's items by the item display order (priority DESC, created_at, id) — matching
// ListByList — and the outer filter keeps the top perList, so the result is capped
// at perList*len(listIDs) rows rather than a full per-list read. ROW_NUMBER() is
// supported by both dialects (modernc SQLite and Postgres).
func (r itemRepo) PreviewsByLists(ctx context.Context, listIDs []string, perList int) (map[string][]storage.ItemPreview, error) {
	out := map[string][]storage.ItemPreview{}
	if len(listIDs) == 0 || perList <= 0 {
		return out, nil
	}

	// Placeholders for the IN list; args are tenant, then each list id, then the
	// per-list cap — the textual order of the ? marks, which is how rebind numbers
	// them for the Postgres dialect.
	ph := make([]string, len(listIDs))
	args := make([]any, 0, len(listIDs)+2)
	args = append(args, r.tenantID)
	for i, id := range listIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	args = append(args, perList)

	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT list_id, id, image_url FROM (
		   SELECT list_id, id, image_url,
		          ROW_NUMBER() OVER (PARTITION BY list_id
		                             ORDER BY priority DESC, created_at, id) AS rn
		     FROM items
		    WHERE tenant_id = ? AND list_id IN (`+strings.Join(ph, ", ")+`)
		      AND archived_at IS NULL
		 ) ranked
		 WHERE rn <= ?
		 ORDER BY list_id, rn`), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			listID string
			p      storage.ItemPreview
			img    sql.NullString
		)
		if err := rows.Scan(&listID, &p.ID, &img); err != nil {
			return nil, err
		}
		if img.Valid {
			s := img.String
			p.ImageURL = &s
		}
		out[listID] = append(out[listID], p)
	}
	return out, rows.Err()
}
