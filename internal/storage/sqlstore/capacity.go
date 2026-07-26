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

// withItemLock runs fn inside a transaction that first locks the item row, so
// concurrent capacity checks on the same item serialize. On Postgres the lock is
// a real SELECT ... FOR UPDATE; on SQLite the single connection already
// serializes writers, so the SELECT just confirms the item exists. Returns
// ErrNotFound if the item is not in the bound tenant.
func (b baseRepo) withItemLock(ctx context.Context, itemID string, fn func(tx *sql.Tx) error) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(ctx, b.rb(
		`SELECT id FROM items WHERE tenant_id = ? AND id = ?`+b.d.forUpdate()),
		b.tenantID, itemID).Scan(&id)
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
