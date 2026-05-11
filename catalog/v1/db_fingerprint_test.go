package catalogv1_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joelanford/library-olm/catalog/fbc"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// expectedFingerprint is the SHA-256 hash of the content schema DDL (from
// sqlite_master) concatenated with a deterministic dump of all content tables
// produced by the FBC fixture below. When the content schema or FBC import
// logic changes, this test will fail. Update ContentSchemaVersion in
// catalog/v1/internal/content.go and refresh this constant.
const expectedFingerprint = "30e730d79b2cbef81d6c50756d81f689c4a8f403cbd23cc13bf34ed6a9366e91"

func TestContentFingerprint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fp.db")
	store, err := catalogv1.OpenStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()

	_, err = store.Set(context.Background(), "fp",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(fbc.NewImporter(fingerprintCatalogFS()), "test"),
	)
	require.NoError(t, err)

	// Open the raw SQLite DB to dump content tables.
	// The "sqlite" driver is registered by the catalogv1 package import.
	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, rawDB.Close()) }()

	var buf strings.Builder

	// 1. Dump content schema DDL from sqlite_master (defines the structure).
	dumpContentSchemaDDL(t, rawDB, &buf)

	// 2. Dump each content table ordered by primary key columns.
	dumpTable(t, rawDB, &buf, "content_schema_version",
		"SELECT version FROM content_schema_version ORDER BY version")
	dumpTable(t, rawDB, &buf, "content_bundles",
		"SELECT catalog_name, bundle_id, package_name, version, release, uri FROM content_bundles ORDER BY catalog_name, bundle_id")
	dumpTable(t, rawDB, &buf, "content_graphs",
		"SELECT catalog_name, name, path, parent_id FROM content_graphs ORDER BY catalog_name, id")
	dumpTable(t, rawDB, &buf, "content_graph_bundles",
		"SELECT graph_id, bundle_id FROM content_graph_bundles ORDER BY graph_id, bundle_id")
	dumpTable(t, rawDB, &buf, "content_successors",
		"SELECT graph_id, from_bundle_id, to_bundle_id FROM content_successors ORDER BY graph_id, from_bundle_id, to_bundle_id")
	dumpTable(t, rawDB, &buf, "content_predecessor_ranges",
		"SELECT graph_id, bundle_id, version_range FROM content_predecessor_ranges ORDER BY graph_id, bundle_id")
	dumpTable(t, rawDB, &buf, "content_bundle_properties",
		"SELECT catalog_name, bundle_id, key, value FROM content_bundle_properties ORDER BY catalog_name, bundle_id, key")
	dumpTable(t, rawDB, &buf, "content_graph_properties",
		"SELECT graph_id, key, value FROM content_graph_properties ORDER BY graph_id, key")

	hash := sha256.Sum256([]byte(buf.String()))
	got := hex.EncodeToString(hash[:])

	assert.Equal(t, expectedFingerprint, got,
		"content fingerprint changed -- bump ContentSchemaVersion in internal/content.go and update expectedFingerprint.\n\nFull dump:\n%s", buf.String())
}

func fingerprintCatalogFS() fstest.MapFS {
	return fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			`{"schema":"olm.package","name":"fp-pkg"}` + "\n" +
				`{"schema":"olm.channel","package":"fp-pkg","name":"stable","entries":[{"name":"fp-pkg.v1.0.0"},{"name":"fp-pkg.v2.0.0","replaces":"fp-pkg.v1.0.0","skipRange":">=1.0.0 <2.0.0"}]}` + "\n" +
				`{"schema":"olm.bundle","package":"fp-pkg","name":"fp-pkg.v1.0.0","image":"quay.io/fp-pkg/bundle:v1.0.0","properties":[{"type":"olm.package","value":{"packageName":"fp-pkg","version":"1.0.0"}}]}` + "\n" +
				`{"schema":"olm.bundle","package":"fp-pkg","name":"fp-pkg.v2.0.0","image":"quay.io/fp-pkg/bundle:v2.0.0","properties":[{"type":"olm.package","value":{"packageName":"fp-pkg","version":"2.0.0"}}]}` + "\n",
		)},
	}
}

// dumpContentSchemaDDL appends the DDL for all content_% tables and indexes
// from sqlite_master, sorted by type then name for determinism.
func dumpContentSchemaDDL(t *testing.T, db *sql.DB, buf *strings.Builder) {
	t.Helper()
	rows, err := db.Query(
		"SELECT type, name, sql FROM sqlite_master WHERE name LIKE 'content_%' AND sql IS NOT NULL ORDER BY type, name",
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	for rows.Next() {
		var objType, name, ddl string
		require.NoError(t, rows.Scan(&objType, &name, &ddl))
		fmt.Fprintf(buf, "DDL|%s|%s|%s\n", objType, name, ddl)
	}
	require.NoError(t, rows.Err())
}

// dumpTable appends a deterministic text representation of all rows from a query
// to buf. Format: "TABLE_NAME|col1|col2|...\n" per row.
func dumpTable(t *testing.T, db *sql.DB, buf *strings.Builder, tableName, query string) {
	t.Helper()
	rows, err := db.Query(query)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	cols, err := rows.Columns()
	require.NoError(t, err)

	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		require.NoError(t, rows.Scan(ptrs...))

		buf.WriteString(tableName)
		for _, v := range values {
			buf.WriteString("|")
			fmt.Fprintf(buf, "%v", v)
		}
		buf.WriteString("\n")
	}
	require.NoError(t, rows.Err())
}
