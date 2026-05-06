package fbc

import (
	"context"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// FromFS walks fsys, parsing FBC blobs via declcfg.WalkMetasFS, and
// loads the structured data into a SQLite database.
//
// If individual packages fail to load or normalize, FromFS returns a
// non-nil Catalog containing only the valid packages alongside an error
// that can be unwrapped into [PackageError] values (one per failed
// package). Fatal errors (corrupt filesystem, database failures) return
// (nil, err).
//
// Deprecated: Use NewImporter with catalogv1.OpenStore and store.Set instead.
func FromFS(ctx context.Context, fsys fs.FS) (*fromFSCatalog, error) {
	tmpDir, err := os.MkdirTemp("", "fbc-fromfs-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	store, err := catalogv1.OpenStore(filepath.Join(tmpDir, "catalog.db"))
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("open store: %w", err)
	}

	imp := NewImporter(fsys)
	if err := store.Set(ctx, "default",
		catalogv1.WithURI("fromfs://"),
		catalogv1.WithContent(imp, "fromfs"),
	); err != nil {
		_ = store.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	cat, err := store.Get("default")
	if err != nil {
		_ = store.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}

	return &fromFSCatalog{
		catalog: cat,
		store:   store,
		tmpDir:  tmpDir,
	}, imp.PackageErrors()
}

type fromFSCatalog struct {
	catalog catalogv1.Catalog
	store   catalogv1.Store
	tmpDir  string
}

func (c *fromFSCatalog) Name() string              { return c.catalog.Name() }
func (c *fromFSCatalog) URI() string                { return c.catalog.URI() }
func (c *fromFSCatalog) Digest() string             { return c.catalog.Digest() }
func (c *fromFSCatalog) Priority() int              { return c.catalog.Priority() }
func (c *fromFSCatalog) Labels() map[string]string   { return c.catalog.Labels() }

func (c *fromFSCatalog) ListPackages(ctx context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	return c.catalog.ListPackages(ctx)
}

func (c *fromFSCatalog) GetPackage(ctx context.Context, name string) (catalogv1.UpdateGraph, error) {
	return c.catalog.GetPackage(ctx, name)
}

// Close releases database resources and removes the temporary file.
func (c *fromFSCatalog) Close() error {
	storeErr := c.store.Close()
	rmErr := os.RemoveAll(c.tmpDir)
	if storeErr != nil {
		return storeErr
	}
	return rmErr
}

var _ catalogv1.Catalog = (*fromFSCatalog)(nil)
