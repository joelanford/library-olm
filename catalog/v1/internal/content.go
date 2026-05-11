package internal

import (
	"database/sql"
	"fmt"
)

// ContentSchemaVersion is the current version of the content schema.
const ContentSchemaVersion = 5

const contentSchemaSQL = `
CREATE TABLE content_schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE content_graphs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_name        TEXT NOT NULL,
    name                TEXT NOT NULL,
    path                TEXT NOT NULL,
    parent_id           INTEGER,
    deprecation_message TEXT CHECK(deprecation_message IS NULL OR length(deprecation_message) > 0),
    FOREIGN KEY (catalog_name) REFERENCES catalog_metadata(name) ON DELETE CASCADE,
    FOREIGN KEY (parent_id) REFERENCES content_graphs(id) ON DELETE CASCADE,
    UNIQUE (catalog_name, path)
);

CREATE TABLE content_bundles (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_name        TEXT NOT NULL,
    bundle_id           TEXT NOT NULL,
    package_name        TEXT NOT NULL DEFAULT '',
    version             TEXT NOT NULL DEFAULT '',
    release             TEXT NOT NULL DEFAULT '',
    uri                 TEXT NOT NULL DEFAULT '',
    deprecation_message TEXT CHECK(deprecation_message IS NULL OR length(deprecation_message) > 0),
    FOREIGN KEY (catalog_name) REFERENCES catalog_metadata(name) ON DELETE CASCADE,
    UNIQUE (catalog_name, bundle_id)
);

CREATE TABLE content_graph_bundles (
    graph_id  INTEGER NOT NULL,
    bundle_id INTEGER NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id) ON DELETE CASCADE,
    FOREIGN KEY (bundle_id) REFERENCES content_bundles(id) ON DELETE CASCADE
);

CREATE TABLE content_successors (
    graph_id       INTEGER NOT NULL,
    from_bundle_id INTEGER NOT NULL,
    to_bundle_id   INTEGER NOT NULL,
    PRIMARY KEY (graph_id, from_bundle_id, to_bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id) ON DELETE CASCADE,
    FOREIGN KEY (from_bundle_id) REFERENCES content_bundles(id) ON DELETE CASCADE,
    FOREIGN KEY (to_bundle_id) REFERENCES content_bundles(id) ON DELETE CASCADE
);

CREATE TABLE content_predecessor_ranges (
    graph_id      INTEGER NOT NULL,
    bundle_id     INTEGER NOT NULL,
    version_range TEXT NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id) ON DELETE CASCADE,
    FOREIGN KEY (bundle_id) REFERENCES content_bundles(id) ON DELETE CASCADE
);

CREATE TABLE content_bundle_properties (
    catalog_name TEXT NOT NULL,
    bundle_id    TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        JSON NOT NULL CHECK(length(value) > 0),
    PRIMARY KEY (catalog_name, bundle_id, key),
    FOREIGN KEY (catalog_name, bundle_id) REFERENCES content_bundles(catalog_name, bundle_id) ON DELETE CASCADE
);

CREATE TABLE content_graph_properties (
    graph_id INTEGER NOT NULL,
    key      TEXT NOT NULL,
    value    JSON NOT NULL CHECK(length(value) > 0),
    PRIMARY KEY (graph_id, key),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id) ON DELETE CASCADE
);

CREATE INDEX idx_content_graphs_parent ON content_graphs(parent_id);
CREATE INDEX idx_content_successors_lookup ON content_successors(graph_id, from_bundle_id);
CREATE INDEX idx_content_predecessor_ranges_lookup ON content_predecessor_ranges(graph_id);
`

// CreateContentTables creates the content schema tables in the database.
func CreateContentTables(db *sql.DB) error {
	_, err := db.Exec(contentSchemaSQL)
	if err != nil {
		return fmt.Errorf("creating content tables: %w", err)
	}
	return nil
}

// contentTablesDropOrder lists content tables in reverse dependency order
// for safe dropping with foreign keys enabled.
var contentTablesDropOrder = []string{
	"content_bundle_properties",
	"content_graph_properties",
	"content_predecessor_ranges",
	"content_successors",
	"content_graph_bundles",
	"content_graphs",
	"content_bundles",
	"content_schema_version",
}

// DropContentTables drops all content_% tables. It verifies that every
// content_% table in the database is accounted for in the known list,
// failing if an unknown table is found. Tables are dropped in reverse
// dependency order so foreign key constraints are not violated.
func DropContentTables(db *sql.DB) error {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'content_%'")
	if err != nil {
		return fmt.Errorf("querying content tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	known := make(map[string]bool, len(contentTablesDropOrder))
	for _, t := range contentTablesDropOrder {
		known[t] = true
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scanning table name: %w", err)
		}
		if !known[name] {
			return fmt.Errorf("unknown content table %q: update contentTablesDropOrder and ContentSchemaVersion", name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table names: %w", err)
	}

	for _, table := range contentTablesDropOrder {
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
