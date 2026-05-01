package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func DeletePackages(ctx context.Context, db *sql.DB, packages map[string]bool) error {
	if len(packages) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cleanup transaction: %w", err)
	}

	for pkg := range packages {
		for _, table := range []string{
			"raw_olm_channel_entry",
			"raw_olm_channel",
			"raw_olm_bundle",
			"raw_olm_package",
		} {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE package_name = ?", table), pkg); err != nil {
				return errors.Join(fmt.Errorf("delete package %q from %s: %w", pkg, table, err), tx.Rollback())
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cleanup transaction: %w", err)
	}
	return nil
}
