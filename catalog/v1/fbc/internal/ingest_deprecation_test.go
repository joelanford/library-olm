package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joelanford/library-olm/catalog/v1/fbc/internal/testing/catalogfs"
)

func TestParseDeprecation_InsertsEntriesWithoutExtension(t *testing.T) {
	writerDB, readerDB, tmpDir, err := OpenTempDB()
	require.NoError(t, err)
	defer func() { require.NoError(t, CloseTempDB(writerDB, readerDB, tmpDir)) }()

	fsys := catalogfs.Builder().
		WithPackage("test-pkg").
		WithChannel("test-pkg", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("test-pkg", "1.0.0").
		WithDeprecation("test-pkg",
			catalogfs.PackageDeprecation("package is deprecated"),
			catalogfs.ChannelDeprecation("stable", "channel is deprecated"),
			catalogfs.BundleDeprecation("test-pkg.v1.0.0", "bundle is deprecated"),
		).
		Build()

	result, err := Ingest(context.Background(), writerDB, fsys, nil)
	require.NoError(t, err)
	require.Empty(t, result.PackageErrors)

	// raw_olm_deprecation should have no rows (ext was nil)
	var depCount int
	require.NoError(t, readerDB.QueryRow("SELECT count(*) FROM "+TableRawDeprecation).Scan(&depCount))
	assert.Equal(t, 0, depCount)

	// raw_olm_deprecation_entries should have 3 rows (unconditional)
	var entryCount int
	require.NoError(t, readerDB.QueryRow("SELECT count(*) FROM "+TableRawDeprecationEntries).Scan(&entryCount))
	assert.Equal(t, 3, entryCount)

	type entry struct {
		packageName string
		schema      string
		name        string
		message     string
	}
	rows, err := readerDB.Query(
		"SELECT package_name, schema, name, message FROM " + TableRawDeprecationEntries + " ORDER BY schema, name",
	)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var entries []entry
	for rows.Next() {
		var e entry
		require.NoError(t, rows.Scan(&e.packageName, &e.schema, &e.name, &e.message))
		entries = append(entries, e)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []entry{
		{packageName: "test-pkg", schema: "olm.bundle", name: "test-pkg.v1.0.0", message: "bundle is deprecated"},
		{packageName: "test-pkg", schema: "olm.channel", name: "stable", message: "channel is deprecated"},
		{packageName: "test-pkg", schema: "olm.package", name: "", message: "package is deprecated"},
	}, entries)
}

func TestParseDeprecation_EmptyMessageRejected(t *testing.T) {
	writerDB, readerDB, tmpDir, err := OpenTempDB()
	require.NoError(t, err)
	defer func() { require.NoError(t, CloseTempDB(writerDB, readerDB, tmpDir)) }()

	tx, err := writerDB.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		"INSERT INTO "+TableRawDeprecationEntries+" (package_name, schema, name, message) VALUES (?, ?, ?, ?)",
		"pkg", "olm.package", "", "",
	)
	assert.Error(t, err, "empty message should be rejected by CHECK constraint")
}
