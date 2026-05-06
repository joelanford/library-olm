package catalogv1_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// testImporter adapts a function into a catalogv1.Importer.
type testImporter struct {
	fn func(ctx context.Context, w catalogv1.Writer) error
}

func (t *testImporter) Import(ctx context.Context, w catalogv1.Writer) error {
	return t.fn(ctx, w)
}

// simpleImporter creates an Importer that inserts one package with one channel
// and one bundle.
func simpleImporter(pkg, version, channel string) catalogv1.Importer {
	return &testImporter{fn: func(_ context.Context, w catalogv1.Writer) error {
		bundleID := pkg + ".v" + version
		if err := w.InsertBundle(bundleID, pkg, version, "", "docker://example.com/"+pkg+":v"+version); err != nil {
			return err
		}
		pkgGraph, err := w.CreateGraph(pkg, nil)
		if err != nil {
			return err
		}
		chGraph, err := w.CreateGraph(channel, &pkgGraph)
		if err != nil {
			return err
		}
		return w.AddBundleToGraph(chGraph, bundleID)
	}}
}

// --- Metadata tests ---

func TestOpenStore_CreatesNewDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "new.db")
	store, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	catalogs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, catalogs)

	_, err = store.Get("nonexistent")
	require.Error(t, err)
}

func TestOpenStore_ExistingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	// First open: insert data
	store1, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	_, err = store1.Set(ctx, "cat1", catalogv1.WithURI("test://one"))
	require.NoError(t, err)
	require.NoError(t, store1.Close())

	// Second open: data should persist
	store2, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, store2.Close()) }()

	cat, err := store2.Get("cat1")
	require.NoError(t, err)
	assert.Equal(t, "cat1", cat.Name())
	assert.Equal(t, "test://one", cat.URI())
}

func TestSet_NewWithURI(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	cat, err := store.Set(ctx, "my-catalog", catalogv1.WithURI("docker://registry.example.com/catalog:latest"))
	require.NoError(t, err)
	assert.Equal(t, "my-catalog", cat.Name())
	assert.Equal(t, "docker://registry.example.com/catalog:latest", cat.URI())
	assert.Equal(t, 0, cat.Priority())
	assert.Empty(t, cat.Digest())
	assert.Empty(t, cat.Labels())
}

func TestSet_NewWithoutURI_Error(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "my-catalog", catalogv1.WithPriority(5))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WithURI is required")

	// Should not be stored
	_, err = store.Get("my-catalog")
	require.Error(t, err)
}

func TestSet_UpdatePartialFields(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create with URI and default priority
	_, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://original"),
	)
	require.NoError(t, err)

	// Update only priority — returned catalog should reflect the update
	cat, err := store.Set(ctx, "cat",
		catalogv1.WithPriority(42),
	)
	require.NoError(t, err)
	assert.Equal(t, "test://original", cat.URI(), "URI should remain unchanged")
	assert.Equal(t, 42, cat.Priority(), "priority should be updated")
}

func TestSet_Labels(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Create with labels
	cat, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithLabels(map[string]string{"env": "prod", "tier": "1"}),
	)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "tier": "1"}, cat.Labels())

	// Update labels: old labels should be replaced entirely
	cat, err = store.Set(ctx, "cat",
		catalogv1.WithLabels(map[string]string{"env": "staging"}),
	)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "staging"}, cat.Labels(),
		"old labels should be removed, only new labels present")

	// Clear labels by setting empty map
	cat, err = store.Set(ctx, "cat",
		catalogv1.WithLabels(map[string]string{}),
	)
	require.NoError(t, err)
	assert.Empty(t, cat.Labels())
}

func TestGet_NotFound(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	_, err := store.Get("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDelete(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	cat, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(simpleImporter("pkg", "1.0.0", "stable"), "digest1"),
	)
	require.NoError(t, err)
	assert.Equal(t, "cat", cat.Name())

	// Delete
	require.NoError(t, store.Delete("cat"))

	// Verify it's gone
	_, err = store.Get("cat")
	require.Error(t, err)

	catalogs, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, catalogs)
}

func TestList(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "charlie", catalogv1.WithURI("test://c"))
	require.NoError(t, err)
	_, err = store.Set(ctx, "alpha", catalogv1.WithURI("test://a"))
	require.NoError(t, err)
	_, err = store.Set(ctx, "bravo", catalogv1.WithURI("test://b"))
	require.NoError(t, err)

	catalogs, err := store.List()
	require.NoError(t, err)
	require.Len(t, catalogs, 3)

	// List returns catalogs ordered by name
	assert.Equal(t, "alpha", catalogs[0].Name())
	assert.Equal(t, "bravo", catalogs[1].Name())
	assert.Equal(t, "charlie", catalogs[2].Name())
}

// --- Content tests ---

func TestSet_WithContent(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	imp := simpleImporter("test-pkg", "1.0.0", "stable")
	cat, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "sha256:abc"),
	)
	require.NoError(t, err)

	var pkgNames []string
	for pkg, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		pkgNames = append(pkgNames, pkg.Name())
	}
	assert.Equal(t, []string{"test-pkg"}, pkgNames)
}

func TestSet_WithContent_Digest(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	imp := simpleImporter("pkg", "1.0.0", "stable")
	cat, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "sha256:deadbeef"),
	)
	require.NoError(t, err)
	assert.Equal(t, "sha256:deadbeef", cat.Digest())
}

func TestSet_WithContent_ReplacesExisting(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// First import
	cat, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(simpleImporter("pkg-a", "1.0.0", "stable"), "digest1"),
	)
	require.NoError(t, err)
	pkgs := collectPackageNames(t, ctx, cat)
	assert.Equal(t, []string{"pkg-a"}, pkgs)

	// Second import replaces first
	cat, err = store.Set(ctx, "cat",
		catalogv1.WithContent(simpleImporter("pkg-b", "2.0.0", "fast"), "digest2"),
	)
	require.NoError(t, err)
	pkgs = collectPackageNames(t, ctx, cat)
	assert.Equal(t, []string{"pkg-b"}, pkgs, "second import should replace first")
	assert.Equal(t, "digest2", cat.Digest())
}

func TestSet_WithContent_RollbackOnError(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// First: successful import
	_, err := store.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(simpleImporter("pkg-a", "1.0.0", "stable"), "digest1"),
	)
	require.NoError(t, err)

	// Second: import that fails mid-way
	failingImporter := &testImporter{fn: func(_ context.Context, w catalogv1.Writer) error {
		if err := w.InsertBundle("fail-pkg.v1.0.0", "fail-pkg", "1.0.0", "", "docker://example.com/fail:v1"); err != nil {
			return err
		}
		return fmt.Errorf("intentional import failure")
	}}

	_, err = store.Set(ctx, "cat",
		catalogv1.WithContent(failingImporter, "digest-fail"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intentional import failure")

	// Original content and metadata should be preserved (transaction rolled back)
	cat, err := store.Get("cat")
	require.NoError(t, err)
	assert.Equal(t, "digest1", cat.Digest(), "digest should remain from first import")

	pkgs := collectPackageNames(t, ctx, cat)
	assert.Equal(t, []string{"pkg-a"}, pkgs, "original content should be preserved")
}

func TestGetContent_NoContent(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Set with URI only, no content
	cat, err := store.Set(ctx, "cat", catalogv1.WithURI("test://"))
	require.NoError(t, err)

	var count int
	for _, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		count++
	}
	assert.Equal(t, 0, count, "catalog with no content should return empty packages")
}

func TestMultiCatalog_Isolation(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Import two catalogs with different content
	catA, err := store.Set(ctx, "catalog-a",
		catalogv1.WithURI("test://a"),
		catalogv1.WithContent(simpleImporter("pkg-a", "1.0.0", "stable"), "digest-a"),
	)
	require.NoError(t, err)
	catB, err := store.Set(ctx, "catalog-b",
		catalogv1.WithURI("test://b"),
		catalogv1.WithContent(simpleImporter("pkg-b", "2.0.0", "fast"), "digest-b"),
	)
	require.NoError(t, err)

	// Catalog A should only see pkg-a
	pkgsA := collectPackageNames(t, ctx, catA)
	assert.Equal(t, []string{"pkg-a"}, pkgsA)

	_, err = catA.GetPackage(ctx, "pkg-b")
	require.Error(t, err, "catalog-a should not see pkg-b")

	// Catalog B should only see pkg-b
	pkgsB := collectPackageNames(t, ctx, catB)
	assert.Equal(t, []string{"pkg-b"}, pkgsB)

	_, err = catB.GetPackage(ctx, "pkg-a")
	require.Error(t, err, "catalog-b should not see pkg-a")
}

// --- Schema lifecycle tests ---

func TestOpenStore_ContentSchemaRebuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "schema.db")
	ctx := context.Background()

	// Open store, import content
	store1, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	_, err = store1.Set(ctx, "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(simpleImporter("pkg", "1.0.0", "stable"), "digest1"),
	)
	require.NoError(t, err)
	require.NoError(t, store1.Close())

	// Tamper with the content_schema_version to force a rebuild.
	// The "sqlite" driver is already registered by the catalogv1 import.
	tamperedDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = tamperedDB.Exec("UPDATE content_schema_version SET version = -1")
	require.NoError(t, err)
	require.NoError(t, tamperedDB.Close())

	// Reopen store: should detect version mismatch, rebuild content tables,
	// and clear digests
	store2, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, store2.Close()) }()

	// Metadata should be preserved
	cat, err := store2.Get("cat")
	require.NoError(t, err)
	assert.Equal(t, "test://", cat.URI())

	// Digest should be cleared (content was rebuilt)
	assert.Empty(t, cat.Digest(), "digest should be cleared after schema rebuild")

	// Content should be gone (tables were dropped and recreated)
	pkgs := collectPackageNames(t, ctx, cat)
	assert.Empty(t, pkgs, "content should be empty after schema rebuild")
}

func TestOpenStore_MetadataMigration_DefaultPriority(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	cat, err := store.Set(ctx, "cat", catalogv1.WithURI("test://"))
	require.NoError(t, err)
	assert.Equal(t, 0, cat.Priority(), "default priority should be 0")
}

// --- Helpers ---

func newTempStore(t *testing.T) (catalogv1.Store, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	return store, func() { require.NoError(t, store.Close()) }
}

func collectPackageNames(t *testing.T, ctx context.Context, cat catalogv1.Catalog) []string {
	t.Helper()
	var names []string
	for pkg, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		names = append(names, pkg.Name())
	}
	return names
}
