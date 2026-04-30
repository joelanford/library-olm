package internal

import (
	"context"
	"database/sql"
	"fmt"
)

func Normalize(ctx context.Context, db *sql.DB, registry *HandlerRegistry) error {
	rows, err := db.QueryContext(ctx, "SELECT name FROM raw_olm_package")
	if err != nil {
		return fmt.Errorf("listing packages: %w", err)
	}
	defer rows.Close()

	var packages []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		packages = append(packages, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, pkgName := range packages {
		handler, err := registry.Get("olm.package")
		if err != nil {
			return fmt.Errorf("package %q: %w", pkgName, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin transaction for %q: %w", pkgName, err)
		}

		if err := handler.Normalize(ctx, tx, pkgName); err != nil {
			tx.Rollback()
			return fmt.Errorf("normalize package %q: %w", pkgName, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit normalization for %q: %w", pkgName, err)
		}
	}
	return nil
}
