package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/fbc"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
)

func deprecationCatalogFS() fstest.MapFS {
	return fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			`{"schema":"olm.package","name":"dep-pkg"}` + "\n" +
				`{"schema":"olm.channel","package":"dep-pkg","name":"stable","entries":[{"name":"dep-pkg.v1.0.0"}]}` + "\n" +
				`{"schema":"olm.channel","package":"dep-pkg","name":"preview","entries":[{"name":"dep-pkg.v1.0.0"}]}` + "\n" +
				`{"schema":"olm.bundle","package":"dep-pkg","name":"dep-pkg.v1.0.0","image":"quay.io/dep-pkg/bundle:v1.0.0","properties":[{"type":"olm.package","value":{"packageName":"dep-pkg","version":"1.0.0"}}]}` + "\n" +
				`{"schema":"olm.deprecations","package":"dep-pkg","entries":[{"reference":{"schema":"olm.package"},"message":"package deprecated"},{"reference":{"schema":"olm.channel","name":"stable"},"message":"stable channel deprecated"},{"reference":{"schema":"olm.bundle","name":"dep-pkg.v1.0.0"},"message":"bundle deprecated"}]}` + "\n" +
				`{"schema":"olm.package","name":"ok-pkg"}` + "\n" +
				`{"schema":"olm.channel","package":"ok-pkg","name":"stable","entries":[{"name":"ok-pkg.v2.0.0"}]}` + "\n" +
				`{"schema":"olm.bundle","package":"ok-pkg","name":"ok-pkg.v2.0.0","image":"quay.io/ok-pkg/bundle:v2.0.0","properties":[{"type":"olm.package","value":{"packageName":"ok-pkg","version":"2.0.0"}}]}` + "\n",
		)},
	}
}

func TestDeprecation_EndToEnd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.OpenStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	_, err = store.Set(context.Background(), "test",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(fbc.NewImporter(deprecationCatalogFS()), "test"),
	)
	require.NoError(t, err)

	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, rawDB.Close()) }()

	t.Run("deprecated package graph", func(t *testing.T) {
		var msg *string
		err := rawDB.QueryRow(
			"SELECT deprecation_message FROM content_graphs WHERE catalog_name = ? AND path = ?",
			"test", "dep-pkg",
		).Scan(&msg)
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Equal(t, "package deprecated", *msg)
	})

	t.Run("deprecated channel graph", func(t *testing.T) {
		var msg *string
		err := rawDB.QueryRow(
			"SELECT deprecation_message FROM content_graphs WHERE catalog_name = ? AND path = ?",
			"test", "dep-pkg/stable",
		).Scan(&msg)
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Equal(t, "stable channel deprecated", *msg)
	})

	t.Run("non-deprecated channel graph has NULL", func(t *testing.T) {
		var msg *string
		err := rawDB.QueryRow(
			"SELECT deprecation_message FROM content_graphs WHERE catalog_name = ? AND path = ?",
			"test", "dep-pkg/preview",
		).Scan(&msg)
		require.NoError(t, err)
		assert.Nil(t, msg)
	})

	t.Run("deprecated bundle", func(t *testing.T) {
		var msg *string
		err := rawDB.QueryRow(
			"SELECT deprecation_message FROM content_bundles WHERE catalog_name = ? AND bundle_id = ?",
			"test", "dep-pkg.v1.0.0",
		).Scan(&msg)
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Equal(t, "bundle deprecated", *msg)
	})

	t.Run("non-deprecated package graph has NULL", func(t *testing.T) {
		var msg *string
		err := rawDB.QueryRow(
			"SELECT deprecation_message FROM content_graphs WHERE catalog_name = ? AND path = ?",
			"test", "ok-pkg",
		).Scan(&msg)
		require.NoError(t, err)
		assert.Nil(t, msg)
	})

	t.Run("non-deprecated bundle has NULL", func(t *testing.T) {
		var msg *string
		err := rawDB.QueryRow(
			"SELECT deprecation_message FROM content_bundles WHERE catalog_name = ? AND bundle_id = ?",
			"test", "ok-pkg.v2.0.0",
		).Scan(&msg)
		require.NoError(t, err)
		assert.Nil(t, msg)
	})

	catalog, err := store.Get("test")
	require.NoError(t, err)

	t.Run("ListPackages deprecated satisfies Deprecated", func(t *testing.T) {
		var foundDep, foundOk bool
		for graph, err := range catalog.ListPackages(context.Background()) {
			require.NoError(t, err)
			switch graph.Name() {
			case "dep-pkg":
				foundDep = true
				d, ok := graph.(catalogv1.Deprecated)
				require.True(t, ok, "deprecated package should satisfy Deprecated")
				assert.Equal(t, "package deprecated", d.DeprecationMessage())
			case "ok-pkg":
				foundOk = true
				_, ok := graph.(catalogv1.Deprecated)
				assert.False(t, ok, "non-deprecated package should not satisfy Deprecated")
			}
		}
		assert.True(t, foundDep, "should have found dep-pkg")
		assert.True(t, foundOk, "should have found ok-pkg")
	})

	t.Run("GetPackage deprecated satisfies Deprecated", func(t *testing.T) {
		graph, err := catalog.GetPackage(context.Background(), "dep-pkg")
		require.NoError(t, err)
		d, ok := graph.(catalogv1.Deprecated)
		require.True(t, ok, "deprecated package graph should satisfy Deprecated")
		assert.Equal(t, "package deprecated", d.DeprecationMessage())
	})

	t.Run("GetPackage deprecated satisfies CompositeUpdateGraph", func(t *testing.T) {
		graph, err := catalog.GetPackage(context.Background(), "dep-pkg")
		require.NoError(t, err)
		_, ok := graph.(catalogv1.CompositeUpdateGraph)
		assert.True(t, ok, "deprecated package graph should still satisfy CompositeUpdateGraph")
	})

	t.Run("ListGraphs deprecated channel satisfies Deprecated", func(t *testing.T) {
		graph, err := catalog.GetPackage(context.Background(), "dep-pkg")
		require.NoError(t, err)
		cug, ok := graph.(catalogv1.CompositeUpdateGraph)
		require.True(t, ok)

		var foundStable, foundPreview bool
		for g, err := range cug.ListGraphs(context.Background()) {
			require.NoError(t, err)
			switch g.Name() {
			case "stable":
				foundStable = true
				d, ok := g.(catalogv1.Deprecated)
				require.True(t, ok, "stable channel should satisfy Deprecated")
				assert.Equal(t, "stable channel deprecated", d.DeprecationMessage())
			case "preview":
				foundPreview = true
				_, ok := g.(catalogv1.Deprecated)
				assert.False(t, ok, "preview channel should not satisfy Deprecated")
			}
		}
		assert.True(t, foundStable, "should have found stable channel")
		assert.True(t, foundPreview, "should have found preview channel")
	})

	t.Run("ListBundles deprecated bundle satisfies Deprecated", func(t *testing.T) {
		graph, err := catalog.GetPackage(context.Background(), "dep-pkg")
		require.NoError(t, err)
		cug, ok := graph.(catalogv1.CompositeUpdateGraph)
		require.True(t, ok)

		stableGraph, err := cug.GetGraph(context.Background(), "stable")
		require.NoError(t, err)

		var found bool
		for b, err := range stableGraph.ListBundles(context.Background()) {
			require.NoError(t, err)
			if string(b.ID()) == "dep-pkg.v1.0.0" {
				found = true
				d, ok := b.(catalogv1.Deprecated)
				require.True(t, ok, "deprecated bundle should satisfy Deprecated")
				assert.Equal(t, "bundle deprecated", d.DeprecationMessage())
			}
		}
		assert.True(t, found, "should have found dep-pkg.v1.0.0")
	})

	t.Run("GetPackage non-deprecated does not satisfy Deprecated", func(t *testing.T) {
		graph, err := catalog.GetPackage(context.Background(), "ok-pkg")
		require.NoError(t, err)
		_, ok := graph.(catalogv1.Deprecated)
		assert.False(t, ok, "non-deprecated package should not satisfy Deprecated")
	})

	t.Run("ListBundles non-deprecated bundle does not satisfy Deprecated", func(t *testing.T) {
		graph, err := catalog.GetPackage(context.Background(), "ok-pkg")
		require.NoError(t, err)
		cug, ok := graph.(catalogv1.CompositeUpdateGraph)
		require.True(t, ok)

		stableGraph, err := cug.GetGraph(context.Background(), "stable")
		require.NoError(t, err)

		for b, err := range stableGraph.ListBundles(context.Background()) {
			require.NoError(t, err)
			_, ok := b.(catalogv1.Deprecated)
			assert.False(t, ok, "non-deprecated bundle %s should not satisfy Deprecated", b.ID())
		}
	})
}

func TestDeprecation_ContentSchemaVersionBump(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.OpenStore(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, rawDB.Close()) }()

	var version int
	err = rawDB.QueryRow("SELECT version FROM content_schema_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 6, version)
}
