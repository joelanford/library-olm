package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropRawTables(t *testing.T) {
	db, tmpDir, err := OpenDB()
	require.NoError(t, err)
	defer func() { require.NoError(t, CloseDB(db, tmpDir)) }()

	ctx := context.Background()

	_, err = db.ExecContext(ctx, `
		INSERT INTO raw_olm_package (package_name) VALUES ('pkg');
		INSERT INTO raw_olm_channel (name, package_name) VALUES ('stable', 'pkg');
		INSERT INTO raw_olm_channel_entry (channel_name, package_name, bundle_name) VALUES ('stable', 'pkg', 'pkg.v1');
		INSERT INTO raw_olm_bundle (name, package_name, version) VALUES ('pkg.v1', 'pkg', '1.0.0');
	`)
	require.NoError(t, err)

	err = DropRawTables(ctx, db)
	require.NoError(t, err)

	for _, table := range RawTables {
		var count int
		err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count)
		assert.ErrorContains(t, err, "no such table", "expected table %s to be dropped", table)
	}
}
