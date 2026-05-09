package internal

import (
	"database/sql"
	"fmt"
)

// MetadataTables lists the tables managed by the metadata schema.
var MetadataTables = []string{
	"catalog_metadata",
	"catalog_labels",
	"metadata_schema_version",
}

// Migration represents a single schema migration with a version number and SQL to execute.
type Migration struct {
	Version int
	SQL     string
}

const migration1SQL = `
CREATE TABLE catalog_metadata (
    name   TEXT PRIMARY KEY,
    uri    TEXT NOT NULL DEFAULT '',
    digest TEXT NOT NULL DEFAULT ''
);

CREATE TABLE catalog_labels (
    catalog_name TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (catalog_name, key),
    FOREIGN KEY (catalog_name) REFERENCES catalog_metadata(name)
);

CREATE TABLE metadata_schema_version (
    version INTEGER NOT NULL
);
`

const migration2SQL = `
ALTER TABLE catalog_metadata ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;
`

const migration3SQL = `
CREATE TABLE catalog_labels_new (
    catalog_name TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (catalog_name, key),
    FOREIGN KEY (catalog_name) REFERENCES catalog_metadata(name) ON DELETE CASCADE
);
INSERT INTO catalog_labels_new SELECT * FROM catalog_labels;
DROP TABLE catalog_labels;
ALTER TABLE catalog_labels_new RENAME TO catalog_labels;
`

// MetadataMigrations is the ordered list of metadata schema migrations.
var MetadataMigrations = []Migration{
	{Version: 1, SQL: migration1SQL},
	{Version: 2, SQL: migration2SQL},
	{Version: 3, SQL: migration3SQL},
}

// RunMetadataMigrations reads the current metadata schema version and applies
// any pending migrations. If the metadata_schema_version table does not exist,
// it starts from version 0.
func RunMetadataMigrations(db *sql.DB) error {
	currentVersion := 0

	exists, err := tableExists(db, "metadata_schema_version")
	if err != nil {
		return fmt.Errorf("checking metadata schema table: %w", err)
	}
	if exists {
		if err := db.QueryRow("SELECT version FROM metadata_schema_version LIMIT 1").Scan(&currentVersion); err != nil {
			return fmt.Errorf("reading metadata schema version: %w", err)
		}
	}

	for _, m := range MetadataMigrations {
		if m.Version <= currentVersion {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			return fmt.Errorf("running metadata migration %d: %w", m.Version, err)
		}
	}

	// Update stored version to the latest migration
	if len(MetadataMigrations) > 0 {
		latestVersion := MetadataMigrations[len(MetadataMigrations)-1].Version
		if currentVersion == 0 {
			// Table was just created by migration 1
			if _, err := db.Exec("INSERT INTO metadata_schema_version (version) VALUES (?)", latestVersion); err != nil {
				return fmt.Errorf("inserting metadata schema version: %w", err)
			}
		} else if latestVersion > currentVersion {
			if _, err := db.Exec("UPDATE metadata_schema_version SET version = ?", latestVersion); err != nil {
				return fmt.Errorf("updating metadata schema version: %w", err)
			}
		}
	}

	return nil
}

// ClearAllDigests resets the digest column for all catalog entries.
func ClearAllDigests(db *sql.DB) error {
	_, err := db.Exec("UPDATE catalog_metadata SET digest = ''")
	if err != nil {
		return fmt.Errorf("clearing digests: %w", err)
	}
	return nil
}
