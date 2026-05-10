package internal

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// OpenTempDB creates a temporary SQLite database with raw FBC staging tables
// and returns separate writer and reader pools. The caller is responsible for
// calling CloseTempDB when done.
func OpenTempDB() (writerDB, readerDB *sql.DB, tmpDir string, err error) {
	tmpDir, err = os.MkdirTemp("", "fbc-staging-*")
	if err != nil {
		return nil, nil, "", fmt.Errorf("creating temp dir: %w", err)
	}

	dbPath := filepath.Join(tmpDir, "staging.db")

	writerDB, err = sql.Open("sqlite", stagingDSN(dbPath, false))
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("opening writer database: %w", err)
	}
	writerDB.SetMaxOpenConns(1)

	if _, err := writerDB.Exec(rawSchemaSQL); err != nil {
		_ = writerDB.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("creating raw tables: %w", err)
	}

	readerDB, err = sql.Open("sqlite", stagingDSN(dbPath, true))
	if err != nil {
		_ = writerDB.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("opening reader database: %w", err)
	}

	return writerDB, readerDB, tmpDir, nil
}

func stagingDSN(path string, readOnly bool) string {
	q := url.Values{}
	if readOnly {
		q.Set("mode", "ro")
	}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	return fmt.Sprintf("file:%s?%s", url.PathEscape(path), q.Encode())
}

// CloseTempDB closes both database pools and removes the temporary directory.
func CloseTempDB(writerDB, readerDB *sql.DB, tmpDir string) error {
	readerErr := readerDB.Close()
	writerErr := writerDB.Close()
	rmErr := os.RemoveAll(tmpDir)
	if readerErr != nil {
		return readerErr
	}
	if writerErr != nil {
		return writerErr
	}
	return rmErr
}

const (
	TableRawPackage      = "raw_olm_package"
	TableRawChannel      = "raw_olm_channel"
	TableRawChannelEntry = "raw_olm_channel_entry"
	TableRawBundle       = "raw_olm_bundle"
	TableRawDeprecation  = "raw_olm_deprecation"
	TableRawOther        = "raw_other"
)

var RawTables = []string{
	TableRawPackage,
	TableRawChannel,
	TableRawChannelEntry,
	TableRawBundle,
	TableRawDeprecation,
	TableRawOther,
}

var rawSchemaSQL = `
-- Raw tables (FBC staging for ingest phase)
-- Table names derived from FBC schema strings: replace "." with "_", prefix "raw_"

CREATE TABLE ` + TableRawPackage + ` (
    package_name TEXT NOT NULL PRIMARY KEY,
    ext_data     JSON
);

CREATE TABLE ` + TableRawChannel + ` (
    name         TEXT NOT NULL,
    package_name TEXT NOT NULL,
    ext_data     JSON,
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
    ext_data     JSON,
    PRIMARY KEY (package_name, name)
);

CREATE TABLE ` + TableRawDeprecation + ` (
    package_name TEXT NOT NULL,
    ext_data     JSON
);

CREATE TABLE ` + TableRawOther + ` (
    schema       TEXT NOT NULL,
    package_name TEXT NOT NULL,
    name         TEXT NOT NULL,
    ext_data     JSON
);
`
