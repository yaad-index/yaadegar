package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// execer is satisfied by both *sql.DB and *sql.Tx, so an INSERT helper can run
// either standalone or inside a capacity transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// withRowLock runs fn inside a transaction that first locks one row of table by
// id, so concurrent mutations of that row serialize. On Postgres the lock is a
// real SELECT ... FOR UPDATE; on SQLite the single connection already serializes
// writers, so the SELECT just confirms the row exists. Returns ErrNotFound if the
// row is not in the bound tenant. `table` is a fixed internal identifier, never
// caller input.
func (b baseRepo) withRowLock(ctx context.Context, table, id string, fn func(tx *sql.Tx) error) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var got string
	err = tx.QueryRowContext(ctx, b.rb(
		`SELECT id FROM `+table+` WHERE tenant_id = ? AND id = ?`+b.d.forUpdate()),
		b.tenantID, id).Scan(&got)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.ErrNotFound
		}
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// withItemLock locks the item row for capacity checks.
func (b baseRepo) withItemLock(ctx context.Context, itemID string, fn func(tx *sql.Tx) error) error {
	return b.withRowLock(ctx, "items", itemID, fn)
}
