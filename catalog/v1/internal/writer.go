package internal

import (
	"database/sql"
	"fmt"

	bsemver "github.com/blang/semver/v4"
)

// ContentWriter writes catalog content into the content tables within a transaction.
type ContentWriter struct {
	tx          *sql.Tx
	catalogName string
}

// NewContentWriter creates a new ContentWriter for the given transaction and catalog name.
func NewContentWriter(tx *sql.Tx, catalogName string) *ContentWriter {
	return &ContentWriter{tx: tx, catalogName: catalogName}
}

// InsertBundle inserts a bundle into the content_bundles table.
// If a bundle with the same ID already exists for this catalog, the insert
// is silently ignored (idempotent for phantom bundles).
func (w *ContentWriter) InsertBundle(id, pkg, version, release, uri string) error {
	_, err := w.tx.Exec(
		"INSERT OR IGNORE INTO content_bundles (catalog_name, bundle_id, package_name, version, release, uri) VALUES (?, ?, ?, ?, ?, ?)",
		w.catalogName, id, pkg, version, release, uri,
	)
	if err != nil {
		return fmt.Errorf("inserting bundle %q: %w", id, err)
	}
	return nil
}

// CreateGraph inserts a graph into the content_graphs table and returns its ID.
func (w *ContentWriter) CreateGraph(name string, parent *int64) (int64, error) {
	var result sql.Result
	var err error
	if parent != nil {
		result, err = w.tx.Exec(
			"INSERT INTO content_graphs (catalog_name, name, parent_id) VALUES (?, ?, ?)",
			w.catalogName, name, *parent,
		)
	} else {
		result, err = w.tx.Exec(
			"INSERT INTO content_graphs (catalog_name, name, parent_id) VALUES (?, ?, NULL)",
			w.catalogName, name,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("creating graph %q: %w", name, err)
	}
	return result.LastInsertId()
}

// AddBundleToGraph associates a bundle with a graph by bundle_id string.
func (w *ContentWriter) AddBundleToGraph(graphID int64, bundleID string) error {
	_, err := w.tx.Exec(
		"INSERT INTO content_graph_bundles (graph_id, bundle_id) SELECT ?, id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?",
		graphID, bundleID, w.catalogName,
	)
	if err != nil {
		return fmt.Errorf("adding bundle %q to graph %d: %w", bundleID, graphID, err)
	}
	return nil
}

// AddEdge adds an explicit successor edge between two bundles in a graph.
// Duplicate edges are silently ignored.
func (w *ContentWriter) AddEdge(graphID int64, fromBundleID, toBundleID string) error {
	_, err := w.tx.Exec(`
		INSERT OR IGNORE INTO content_successors (graph_id, from_bundle_id, to_bundle_id)
		VALUES (?,
			(SELECT id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?),
			(SELECT id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?))`,
		graphID, fromBundleID, w.catalogName, toBundleID, w.catalogName,
	)
	if err != nil {
		return fmt.Errorf("adding edge %q -> %q in graph %d: %w", fromBundleID, toBundleID, graphID, err)
	}
	return nil
}

func (w *ContentWriter) AddPredecessorRange(graphID int64, bundleID, versionRange string) error {
	if _, err := bsemver.ParseRange(versionRange); err != nil {
		return fmt.Errorf("invalid predecessor range %q for bundle %q: %w", versionRange, bundleID, err)
	}
	_, err := w.tx.Exec(`
		INSERT INTO content_predecessor_ranges (graph_id, bundle_id, version_range)
		VALUES (?, (SELECT id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?), ?)`,
		graphID, bundleID, w.catalogName, versionRange,
	)
	if err != nil {
		return fmt.Errorf("adding predecessor range for bundle %q in graph %d: %w", bundleID, graphID, err)
	}
	return nil
}

// DeleteCatalogContent deletes all content rows for a catalog name,
// respecting foreign key ordering.
func DeleteCatalogContent(tx *sql.Tx, catalogName string) error {
	// Delete in FK-safe order: predecessor_ranges, successors, graph_bundles, graphs, bundles
	stmts := []string{
		"DELETE FROM content_predecessor_ranges WHERE graph_id IN (SELECT id FROM content_graphs WHERE catalog_name = ?)",
		"DELETE FROM content_successors WHERE graph_id IN (SELECT id FROM content_graphs WHERE catalog_name = ?)",
		"DELETE FROM content_graph_bundles WHERE graph_id IN (SELECT id FROM content_graphs WHERE catalog_name = ?)",
		"DELETE FROM content_graphs WHERE catalog_name = ?",
		"DELETE FROM content_bundles WHERE catalog_name = ?",
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt, catalogName); err != nil {
			return fmt.Errorf("deleting catalog content for %q: %w", catalogName, err)
		}
	}
	return nil
}
