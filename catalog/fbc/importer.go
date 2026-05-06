package fbc

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/joelanford/library-olm/catalog/fbc/internal"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// Importer imports FBC (File-Based Catalog) content into a store via a Writer.
type Importer struct {
	fsys fs.FS
}

// NewImporter creates a new FBC importer that reads FBC data from fsys.
func NewImporter(fsys fs.FS) *Importer {
	return &Importer{fsys: fsys}
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
	rawDB, tmpDir, err := internal.OpenTempDB()
	if err != nil {
		return fmt.Errorf("open staging database: %w", err)
	}
	defer func() { _ = internal.CloseTempDB(rawDB, tmpDir) }()

	ingestResult, err := internal.Ingest(ctx, rawDB, i.fsys)
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	skipPackages := make(map[string]bool, len(ingestResult.PackageErrors))
	for pkg := range ingestResult.PackageErrors {
		skipPackages[pkg] = true
	}

	registry := internal.NewHandlerRegistry()
	registry.Register(&internal.OLMPackageHandler{})

	normalizeResult, err := internal.Normalize(ctx, rawDB, registry, skipPackages, w)
	if err != nil {
		return fmt.Errorf("normalize: %w", err)
	}

	return mergePackageErrors(ingestResult.PackageErrors, normalizeResult.PackageErrors)
}

// Compile-time check that *Importer implements catalogv1.Importer.
var _ catalogv1.Importer = (*Importer)(nil)
