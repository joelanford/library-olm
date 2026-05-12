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
	assert.Equal(t, 5, version)
}
