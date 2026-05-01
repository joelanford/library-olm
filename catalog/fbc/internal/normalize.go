package internal

import (
	"context"
	"database/sql"
	"fmt"
)

type NormalizeResult struct {
	PackageErrors map[string][]error
}

func Normalize(ctx context.Context, db *sql.DB, registry *HandlerRegistry, skipPackages map[string]bool) (*NormalizeResult, error) {
	rows, err := db.QueryContext(ctx, "SELECT package_name FROM raw_olm_package")
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var packages []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		packages = append(packages, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pkgErrors := make(map[string][]error)
	for _, pkgName := range packages {
		if skipPackages[pkgName] {
			continue
		}

		handler, err := registry.Get("olm.package")
		if err != nil {
			return nil, fmt.Errorf("package %q: %w", pkgName, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("begin transaction for %q: %w", pkgName, err)
		}

		if err := handler.Normalize(ctx, tx, pkgName); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return nil, fmt.Errorf("rollback for %q: %w", pkgName, rbErr)
			}
			pkgErrors[pkgName] = append(pkgErrors[pkgName], fmt.Errorf("normalize: %w", err))
			continue
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit normalization for %q: %w", pkgName, err)
		}
	}
	return &NormalizeResult{PackageErrors: pkgErrors}, nil
}
