package internal

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func OpenDB() (*sql.DB, string, error) {
	tmpDir, err := os.MkdirTemp("", "fbc-catalog-*")
	if err != nil {
		return nil, "", fmt.Errorf("creating temp dir: %w", err)
	}

	dbPath := filepath.Join(tmpDir, "catalog.db")
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

	if err := createTables(db); err != nil {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("creating tables: %w", err)
	}

	return db, tmpDir, nil
}

func CloseDB(db *sql.DB, tmpDir string) error {
	dbErr := db.Close()
	rmErr := os.RemoveAll(tmpDir)
	if dbErr != nil {
		return dbErr
	}
	return rmErr
}

func createTables(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

const schemaSQL = `
-- Raw tables (phase 1 ingest)
-- Table names derived from FBC schema strings: replace "." with "_", prefix "raw_"

CREATE TABLE raw_olm_package (
    package_name TEXT NOT NULL PRIMARY KEY
);

CREATE TABLE raw_olm_channel (
    name         TEXT NOT NULL,
    package_name TEXT NOT NULL,
    PRIMARY KEY (package_name, name)
);

CREATE TABLE raw_olm_channel_entry (
    channel_name TEXT NOT NULL,
    package_name TEXT NOT NULL,
    bundle_name  TEXT NOT NULL,
    replaces     TEXT NOT NULL DEFAULT '',
    skips        TEXT NOT NULL DEFAULT '',
    skip_range   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (package_name, channel_name, bundle_name)
);

CREATE TABLE raw_olm_bundle (
    name         TEXT NOT NULL,
    package_name TEXT NOT NULL,
    version      TEXT NOT NULL,
    release      TEXT NOT NULL DEFAULT '',
    image        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (package_name, name)
);

-- Normalized tables (phase 2 handlers)

CREATE TABLE graphs (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    parent_id INTEGER,
    FOREIGN KEY (parent_id) REFERENCES graphs(id),
    UNIQUE (name, parent_id)
);

CREATE TABLE bundles (
    id           TEXT NOT NULL PRIMARY KEY,
    package_name TEXT NOT NULL DEFAULT '',
    version      TEXT NOT NULL DEFAULT '',
    release      TEXT NOT NULL DEFAULT '',
    uri          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE graph_bundles (
    graph_id  INTEGER NOT NULL,
    bundle_id TEXT NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES graphs(id),
    FOREIGN KEY (bundle_id) REFERENCES bundles(id)
);

CREATE TABLE successors (
    graph_id       INTEGER NOT NULL,
    from_bundle_id TEXT NOT NULL,
    to_bundle_id   TEXT NOT NULL,
    PRIMARY KEY (graph_id, from_bundle_id, to_bundle_id),
    FOREIGN KEY (graph_id) REFERENCES graphs(id),
    FOREIGN KEY (from_bundle_id) REFERENCES bundles(id),
    FOREIGN KEY (to_bundle_id) REFERENCES bundles(id)
);

-- Indexes for common query patterns
CREATE INDEX idx_graphs_parent ON graphs(parent_id);
CREATE INDEX idx_successors_lookup ON successors(graph_id, from_bundle_id);
`
