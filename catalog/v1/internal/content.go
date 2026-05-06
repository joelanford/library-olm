package internal

import (
	"database/sql"
	"fmt"
)

// ContentSchemaVersion is the current version of the content schema.
const ContentSchemaVersion = 1

const contentSchemaSQL = `
CREATE TABLE content_schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE content_graphs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_name TEXT NOT NULL,
    name         TEXT NOT NULL,
    parent_id    INTEGER,
    FOREIGN KEY (parent_id) REFERENCES content_graphs(id),
    UNIQUE (catalog_name, name, parent_id)
);

CREATE TABLE content_bundles (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_name TEXT NOT NULL,
    bundle_id    TEXT NOT NULL,
    package_name TEXT NOT NULL DEFAULT '',
    version      TEXT NOT NULL DEFAULT '',
    release      TEXT NOT NULL DEFAULT '',
    uri          TEXT NOT NULL DEFAULT '',
    UNIQUE (catalog_name, bundle_id)
);

CREATE TABLE content_graph_bundles (
    graph_id  INTEGER NOT NULL,
    bundle_id INTEGER NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id),
    FOREIGN KEY (bundle_id) REFERENCES content_bundles(id)
);

CREATE TABLE content_successors (
    graph_id       INTEGER NOT NULL,
    from_bundle_id INTEGER NOT NULL,
    to_bundle_id   INTEGER NOT NULL,
    PRIMARY KEY (graph_id, from_bundle_id, to_bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id),
    FOREIGN KEY (from_bundle_id) REFERENCES content_bundles(id),
    FOREIGN KEY (to_bundle_id) REFERENCES content_bundles(id)
);

CREATE INDEX idx_content_graphs_parent ON content_graphs(parent_id);
CREATE INDEX idx_content_successors_lookup ON content_successors(graph_id, from_bundle_id);
`

// CreateContentTables creates the content schema tables in the database.
func CreateContentTables(db *sql.DB) error {
	_, err := db.Exec(contentSchemaSQL)
	if err != nil {
		return fmt.Errorf("creating content tables: %w", err)
	}
	return nil
}

// DropContentTables drops all tables matching the content_% prefix.
func DropContentTables(db *sql.DB) error {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'content_%'")
	if err != nil {
		return fmt.Errorf("querying content tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scanning table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table names: %w", err)
	}

	for _, table := range tables {
		if _, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q", table)); err != nil {
			return fmt.Errorf("dropping table %q: %w", table, err)
		}
	}
	return nil
}

// CheckContentSchemaVersion returns true if the stored content schema version
// matches ContentSchemaVersion. If the content_schema_version table does not
// exist, it returns false with no error (triggering a rebuild).
func CheckContentSchemaVersion(db *sql.DB) (bool, error) {
	exists, err := tableExists(db, "content_schema_version")
	if err != nil {
		return false, fmt.Errorf("checking content schema table: %w", err)
	}
	if !exists {
		return false, nil
	}
	var version int
	if err := db.QueryRow("SELECT version FROM content_schema_version LIMIT 1").Scan(&version); err != nil {
		return false, fmt.Errorf("querying content schema version: %w", err)
	}
	return version == ContentSchemaVersion, nil
}

// StoreContentSchemaVersion inserts the current content schema version into
// the content_schema_version table.
func StoreContentSchemaVersion(db *sql.DB) error {
	_, err := db.Exec("INSERT INTO content_schema_version (version) VALUES (?)", ContentSchemaVersion)
	if err != nil {
		return fmt.Errorf("storing content schema version: %w", err)
	}
	return nil
}
