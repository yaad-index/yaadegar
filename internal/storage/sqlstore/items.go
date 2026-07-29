package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type itemRepo struct{ baseRepo }

const itemCols = `id, tenant_id, list_id, name, url, image_url,
	price_amount_minor, price_currency, note, priority, quantity_wanted, created_at`

func scanItem(s scanner) (storage.Item, error) {
	var (
		it        storage.Item
		url       sql.NullString
		imageURL  sql.NullString
		amount    sql.NullInt64
		currency  sql.NullString
		note      sql.NullString
		createdAt string
	)
	if err := s.Scan(&it.ID, &it.TenantID, &it.ListID, &it.Name, &url, &imageURL,
		&amount, &currency, &note, &it.Priority, &it.QuantityWanted, &createdAt); err != nil {
		return storage.Item{}, err
	}
	ts, err := parseTime(createdAt)
	if err != nil {
		return storage.Item{}, err
	}
	it.URL = strPtr(url)
	it.ImageURL = strPtr(imageURL)
	it.Price = pricePtr(amount, currency)
	it.Note = strPtr(note)
	it.CreatedAt = ts
	return it, nil
}

func (r itemRepo) Create(ctx context.Context, it storage.Item) (storage.Item, error) {
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
	amount, currency := priceCols(it.Price)

	_, err := r.db.ExecContext(ctx, r.rb(
		`INSERT INTO items (`+itemCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		it.ID, it.TenantID, it.ListID, it.Name, nullStr(it.URL), nullStr(it.ImageURL),
		amount, currency, nullStr(it.Note), it.Priority, it.QuantityWanted,
		fmtTime(it.CreatedAt))
	if err != nil {
		return storage.Item{}, err
	}
	return it, nil
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

func (r itemRepo) ListByList(ctx context.Context, listID string, p storage.Page) ([]storage.Item, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, r.rb(
		`SELECT COUNT(*) FROM items WHERE tenant_id = ? AND list_id = ?`),
		r.tenantID, listID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, r.rb(
		`SELECT `+itemCols+` FROM items
		  WHERE tenant_id = ? AND list_id = ?
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
		        price_currency = ?, note = ?, priority = ?, quantity_wanted = ?
		  WHERE tenant_id = ? AND id = ?`),
		it.Name, nullStr(it.URL), nullStr(it.ImageURL), amount, currency,
		nullStr(it.Note), it.Priority, it.QuantityWanted, r.tenantID, it.ID)
	if err != nil {
		return storage.Item{}, err
	}
	if err := expectOne(res); err != nil {
		return storage.Item{}, err
	}
	return r.Get(ctx, it.ID)
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
