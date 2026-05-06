package internal

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// OpenTempDB creates a temporary SQLite database with raw FBC staging tables.
// The caller is responsible for calling CloseTempDB when done.
func OpenTempDB() (*sql.DB, string, error) {
	tmpDir, err := os.MkdirTemp("", "fbc-staging-*")
	if err != nil {
		return nil, "", fmt.Errorf("creating temp dir: %w", err)
	}

	dbPath := filepath.Join(tmpDir, "staging.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;
	`); err != nil {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("setting pragmas: %w", err)
	}

	if _, err := db.Exec(rawSchemaSQL); err != nil {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("creating raw tables: %w", err)
	}

	return db, tmpDir, nil
}

// CloseTempDB closes the database and removes the temporary directory.
func CloseTempDB(db *sql.DB, tmpDir string) error {
	dbErr := db.Close()
	rmErr := os.RemoveAll(tmpDir)
	if dbErr != nil {
		return dbErr
	}
	return rmErr
}

const (
	TableRawPackage      = "raw_olm_package"
	TableRawChannel      = "raw_olm_channel"
	TableRawChannelEntry = "raw_olm_channel_entry"
	TableRawBundle       = "raw_olm_bundle"
)

var RawTables = []string{
	TableRawPackage,
	TableRawChannel,
	TableRawChannelEntry,
	TableRawBundle,
}

var rawSchemaSQL = `
-- Raw tables (FBC staging for ingest phase)
-- Table names derived from FBC schema strings: replace "." with "_", prefix "raw_"

CREATE TABLE ` + TableRawPackage + ` (
    package_name TEXT NOT NULL PRIMARY KEY
);

CREATE TABLE ` + TableRawChannel + ` (
    name         TEXT NOT NULL,
    package_name TEXT NOT NULL,
    PRIMARY KEY (package_name, name)
);

CREATE TABLE ` + TableRawChannelEntry + ` (
    channel_name TEXT NOT NULL,
    package_name TEXT NOT NULL,
    bundle_name  TEXT NOT NULL,
    replaces     TEXT NOT NULL DEFAULT '',
    skips        TEXT NOT NULL DEFAULT '',
    skip_range   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (package_name, channel_name, bundle_name)
);

CREATE TABLE ` + TableRawBundle + ` (
    name         TEXT NOT NULL,
    package_name TEXT NOT NULL,
    version      TEXT NOT NULL,
    release      TEXT NOT NULL DEFAULT '',
    image        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (package_name, name)
);
`
