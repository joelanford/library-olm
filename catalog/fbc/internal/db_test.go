package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenTempDB(t *testing.T) {
	db, tmpDir, err := OpenTempDB()
	require.NoError(t, err)
	defer func() { require.NoError(t, CloseTempDB(db, tmpDir)) }()

	var count int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	require.NoError(t, err)
	// Raw tables only: 4 tables
	require.GreaterOrEqual(t, count, 4)
}
