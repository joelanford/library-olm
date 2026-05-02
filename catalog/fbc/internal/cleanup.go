package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func DropRawTables(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin drop raw tables transaction: %w", err)
	}
	for _, table := range RawTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return errors.Join(fmt.Errorf("drop table %s: %w", table, err), tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit drop raw tables transaction: %w", err)
	}
	return nil
}
