package fbc

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"iter"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc/internal"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// Catalog is a catalogv1.Catalog backed by FBC data stored in SQLite.
// Call Close when done to clean up the temporary database file.
type Catalog struct {
	db     *sql.DB
	tmpDir string
	query  *internal.CatalogQuery
}

// FromFS walks fsys, parsing FBC blobs via declcfg.WalkMetasFS, and
// loads the structured data into a SQLite database. After loading,
// it dispatches per-package-schema handlers that validate and compute
// successor graphs into normalized tables.
//
// If individual packages fail to load or normalize, FromFS returns a
// non-nil Catalog containing only the valid packages alongside an error
// that can be unwrapped into [PackageError] values (one per failed
// package). Fatal errors (corrupt filesystem, database failures) return
// (nil, err).
//
// The database is stored in a temporary file. Call Close to remove it.
func FromFS(ctx context.Context, fsys fs.FS) (*Catalog, error) {
	db, tmpDir, err := internal.OpenDB()
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	ingestResult, err := internal.Ingest(ctx, db, fsys)
	if err != nil {
		_ = internal.CloseDB(db, tmpDir)
		return nil, fmt.Errorf("ingest: %w", err)
	}

	skipPackages := make(map[string]bool, len(ingestResult.PackageErrors))
	for pkg := range ingestResult.PackageErrors {
		skipPackages[pkg] = true
	}

	registry := internal.NewHandlerRegistry()
	registry.Register(&internal.OLMPackageHandler{})

	normalizeResult, err := internal.Normalize(ctx, db, registry, skipPackages)
	if err != nil {
		_ = internal.CloseDB(db, tmpDir)
		return nil, fmt.Errorf("normalize: %w", err)
	}

	for pkg := range normalizeResult.PackageErrors {
		skipPackages[pkg] = true
	}
	if err := internal.DropRawTables(ctx, db); err != nil {
		_ = internal.CloseDB(db, tmpDir)
		return nil, fmt.Errorf("drop raw tables: %w", err)
	}

	pkgErr := mergePackageErrors(ingestResult.PackageErrors, normalizeResult.PackageErrors)

	return &Catalog{
		db:     db,
		tmpDir: tmpDir,
		query:  &internal.CatalogQuery{DB: db},
	}, pkgErr
}

// Close releases database resources and removes the temporary file.
func (c *Catalog) Close() error {
	return internal.CloseDB(c.db, c.tmpDir)
}

func (c *Catalog) ListPackages(ctx context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	return c.query.ListPackages(ctx)
}

func (c *Catalog) GetPackage(ctx context.Context, name string) (catalogv1.UpdateGraph, error) {
	return c.query.GetPackage(ctx, name)
}

// Ensure Catalog satisfies catalogv1.Catalog at compile time.
var _ catalogv1.Catalog = (*Catalog)(nil)

// Ensure query types satisfy their interfaces at compile time.
var _ catalogv1.CompositeUpdateGraph = (*internal.CompositeUpdateGraphQuery)(nil)
var _ catalogv1.UpdateGraph = (*internal.UpdateGraphQuery)(nil)

// Ensure BundleRow satisfies bundlev1.Bundle at compile time.
var _ bundlev1.Bundle = internal.BundleRow{}
