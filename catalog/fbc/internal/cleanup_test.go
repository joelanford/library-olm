package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletePackages(t *testing.T) {
	db, tmpDir, err := OpenDB()
	require.NoError(t, err)
	defer func() { require.NoError(t, CloseDB(db, tmpDir)) }()

	ctx := context.Background()

	_, err = db.ExecContext(ctx, `
		INSERT INTO raw_olm_package (package_name) VALUES ('keep'), ('drop');
		INSERT INTO raw_olm_channel (name, package_name) VALUES ('stable', 'keep'), ('stable', 'drop');
		INSERT INTO raw_olm_channel_entry (channel_name, package_name, bundle_name) VALUES ('stable', 'keep', 'keep.v1'), ('stable', 'drop', 'drop.v1');
		INSERT INTO raw_olm_bundle (name, package_name, version) VALUES ('keep.v1', 'keep', '1.0.0'), ('drop.v1', 'drop', '1.0.0');
	`)
	require.NoError(t, err)

	err = DeletePackages(ctx, db, map[string]bool{"drop": true})
	require.NoError(t, err)

	for _, q := range []string{
		"SELECT count(*) FROM raw_olm_package WHERE package_name = 'drop'",
		"SELECT count(*) FROM raw_olm_channel WHERE package_name = 'drop'",
		"SELECT count(*) FROM raw_olm_channel_entry WHERE package_name = 'drop'",
		"SELECT count(*) FROM raw_olm_bundle WHERE package_name = 'drop'",
	} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx, q).Scan(&count))
		assert.Equal(t, 0, count, q)
	}

	for _, q := range []string{
		"SELECT count(*) FROM raw_olm_package WHERE package_name = 'keep'",
		"SELECT count(*) FROM raw_olm_channel WHERE package_name = 'keep'",
		"SELECT count(*) FROM raw_olm_channel_entry WHERE package_name = 'keep'",
		"SELECT count(*) FROM raw_olm_bundle WHERE package_name = 'keep'",
	} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx, q).Scan(&count))
		assert.Equal(t, 1, count, q)
	}
}

func TestDeletePackages_Empty(t *testing.T) {
	db, tmpDir, err := OpenDB()
	require.NoError(t, err)
	defer func() { require.NoError(t, CloseDB(db, tmpDir)) }()

	err = DeletePackages(context.Background(), db, nil)
	require.NoError(t, err)

	err = DeletePackages(context.Background(), db, map[string]bool{})
	require.NoError(t, err)
}
