package internal

import (
	"context"
	"database/sql"
	"fmt"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

type NormalizeResult struct {
	PackageErrors map[string][]error
}

func Normalize(ctx context.Context, rawDB *sql.DB, registry *HandlerRegistry, skipPackages map[string]bool, w catalogv1.Writer) (*NormalizeResult, error) {
	rows, err := rawDB.QueryContext(ctx, "SELECT package_name FROM "+TableRawPackage)
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

		if err := handler.Normalize(ctx, rawDB, w, pkgName); err != nil {
			pkgErrors[pkgName] = append(pkgErrors[pkgName], fmt.Errorf("normalize: %w", err))
			continue
		}
	}
	return &NormalizeResult{PackageErrors: pkgErrors}, nil
}
