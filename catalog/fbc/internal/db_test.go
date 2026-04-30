package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenDB(t *testing.T) {
	db, tmpDir, err := OpenDB()
	require.NoError(t, err)
	defer CloseDB(db, tmpDir)

	var count int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	require.NoError(t, err)
	require.GreaterOrEqual(t, count, 8)
}
