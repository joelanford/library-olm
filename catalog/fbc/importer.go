package fbc

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/joelanford/library-olm/catalog/fbc/internal"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// Importer imports FBC (File-Based Catalog) content into a store via a Writer.
type Importer struct {
	fsys      fs.FS
	olmPkgExt OLMPackageExtension
}

// NewImporter creates a new FBC importer that reads FBC data from fsys.
func NewImporter(fsys fs.FS, opts ...ImporterOption) *Importer {
	imp := &Importer{fsys: fsys}
	for _, opt := range opts {
		opt(imp)
	}
	return imp
}

// Import reads FBC blobs from the filesystem, ingests them into a temporary
// staging database, normalizes per-package content, and writes the results
// through w. Valid packages are imported even when other packages fail.
//
// Per-package errors (malformed bundles, invalid skip ranges, etc.) are
// returned as a [catalogv1.PartialImportError] that can be unwrapped into
// individual [PackageError] values. Fatal errors (corrupt filesystem,
// database failures) are returned directly.
func (i *Importer) Import(ctx context.Context, w catalogv1.Writer) error {
	writerDB, readerDB, tmpDir, err := internal.OpenTempDB()
	if err != nil {
		return fmt.Errorf("open staging database: %w", err)
	}
	defer func() { _ = internal.CloseTempDB(writerDB, readerDB, tmpDir) }()

	ingestResult, err := internal.Ingest(ctx, writerDB, i.fsys, i.olmPkgExt)
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	skipPackages := make(map[string]bool, len(ingestResult.PackageErrors))
	for pkg := range ingestResult.PackageErrors {
		skipPackages[pkg] = true
	}

	registry := internal.NewHandlerRegistry()
	registry.Register(&internal.OLMPackageHandler{})

	normalizeResult, err := internal.Normalize(ctx, readerDB, registry, skipPackages, w)
	if err != nil {
		return fmt.Errorf("normalize: %w", err)
	}

	for pkg := range normalizeResult.PackageErrors {
		skipPackages[pkg] = true
	}

	var finalizeErrors map[string][]error
	if i.olmPkgExt != nil {
		finalizeErrors, err = i.finalize(ctx, readerDB, w, skipPackages)
		if err != nil {
			return fmt.Errorf("finalize: %w", err)
		}
	}

	return mergePackageErrors(ingestResult.PackageErrors, normalizeResult.PackageErrors, finalizeErrors)
}

func (i *Importer) finalize(ctx context.Context, readerDB *sql.DB, w catalogv1.Writer, skipPackages map[string]bool) (map[string][]error, error) {
	rows, err := readerDB.QueryContext(ctx, "SELECT package_name FROM "+internal.TableRawPackage)
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
		pkg := &packageAccessorAdapter{a: internal.NewPackageAccessor(readerDB, pkgName)}
		pw := internal.NewPropertyWriter(pkgName, w)
		if err := i.olmPkgExt.FinalizePackage(ctx, pkg, pw); err != nil {
			pkgErrors[pkgName] = append(pkgErrors[pkgName], fmt.Errorf("finalize: %w", err))
		}
	}
	return pkgErrors, nil
}

// Compile-time check that *Importer implements catalogv1.Importer.
var _ catalogv1.Importer = (*Importer)(nil)
