package internal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	bsemver "github.com/blang/semver/v4"
)

func graphPath(path []string) (string, error) {
	for _, seg := range path {
		if seg == "" || strings.Contains(seg, "/") {
			return "", fmt.Errorf("invalid graph path segment %q", seg)
		}
	}
	return strings.Join(path, "/"), nil
}

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

func (w *ContentWriter) resolveGraphID(path []string) (int64, error) {
	if len(path) == 0 {
		return 0, fmt.Errorf("empty graph path")
	}
	p, err := graphPath(path)
	if err != nil {
		return 0, err
	}
	var graphID int64
	err = w.tx.QueryRow(
		"SELECT id FROM content_graphs WHERE catalog_name = ? AND path = ?",
		w.catalogName, p,
	).Scan(&graphID)
	if err != nil {
		return 0, fmt.Errorf("resolve graph path %v: %w", path, err)
	}
	return graphID, nil
}

func (w *ContentWriter) CreateGraph(path []string) error {
	if len(path) == 0 {
		return fmt.Errorf("empty graph path")
	}
	name := path[len(path)-1]
	p, err := graphPath(path)
	if err != nil {
		return err
	}
	if len(path) == 1 {
		_, err := w.tx.Exec(
			"INSERT INTO content_graphs (catalog_name, name, path, parent_id) VALUES (?, ?, ?, NULL)",
			w.catalogName, name, p,
		)
		if err != nil {
			return fmt.Errorf("creating graph %v: %w", path, err)
		}
		return nil
	}
	parentID, err := w.resolveGraphID(path[:len(path)-1])
	if err != nil {
		return fmt.Errorf("creating graph %v: %w", path, err)
	}
	_, err = w.tx.Exec(
		"INSERT INTO content_graphs (catalog_name, name, path, parent_id) VALUES (?, ?, ?, ?)",
		w.catalogName, name, p, parentID,
	)
	if err != nil {
		return fmt.Errorf("creating graph %v: %w", path, err)
	}
	return nil
}

func (w *ContentWriter) AddBundleToGraph(path []string, bundleID string) error {
	graphID, err := w.resolveGraphID(path)
	if err != nil {
		return fmt.Errorf("adding bundle %q to graph %v: %w", bundleID, path, err)
	}
	_, err = w.tx.Exec(
		"INSERT INTO content_graph_bundles (graph_id, bundle_id) SELECT ?, id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?",
		graphID, bundleID, w.catalogName,
	)
	if err != nil {
		return fmt.Errorf("adding bundle %q to graph %v: %w", bundleID, path, err)
	}
	return nil
}

func (w *ContentWriter) AddEdge(path []string, fromBundleID, toBundleID string) error {
	graphID, err := w.resolveGraphID(path)
	if err != nil {
		return fmt.Errorf("adding edge %q -> %q in graph %v: %w", fromBundleID, toBundleID, path, err)
	}
	_, err = w.tx.Exec(`
		INSERT OR IGNORE INTO content_successors (graph_id, from_bundle_id, to_bundle_id)
		VALUES (?,
			(SELECT id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?),
			(SELECT id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?))`,
		graphID, fromBundleID, w.catalogName, toBundleID, w.catalogName,
	)
	if err != nil {
		return fmt.Errorf("adding edge %q -> %q in graph %v: %w", fromBundleID, toBundleID, path, err)
	}
	return nil
}

func (w *ContentWriter) AddPredecessorRange(path []string, bundleID, versionRange string) error {
	if _, err := bsemver.ParseRange(versionRange); err != nil {
		return fmt.Errorf("invalid predecessor range %q for bundle %q: %w", versionRange, bundleID, err)
	}
	graphID, err := w.resolveGraphID(path)
	if err != nil {
		return fmt.Errorf("adding predecessor range for bundle %q in graph %v: %w", bundleID, path, err)
	}
	_, err = w.tx.Exec(`
		INSERT INTO content_predecessor_ranges (graph_id, bundle_id, version_range)
		VALUES (?, (SELECT id FROM content_bundles WHERE bundle_id = ? AND catalog_name = ?), ?)`,
		graphID, bundleID, w.catalogName, versionRange,
	)
	if err != nil {
		return fmt.Errorf("adding predecessor range for bundle %q in graph %v: %w", bundleID, path, err)
	}
	return nil
}

func (w *ContentWriter) SetBundleProperty(bundleID, key string, val any) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal bundle property %q on %q: %w", key, bundleID, err)
	}
	_, err = w.tx.Exec(
		"INSERT OR REPLACE INTO content_bundle_properties (catalog_name, bundle_id, key, value) VALUES (?, ?, ?, ?)",
		w.catalogName, bundleID, key, string(data),
	)
	if err != nil {
		return fmt.Errorf("setting bundle property %q on %q: %w", key, bundleID, err)
	}
	return nil
}

func (w *ContentWriter) SetGraphProperty(path []string, key string, val any) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal graph property %q on path %v: %w", key, path, err)
	}
	graphID, err := w.resolveGraphID(path)
	if err != nil {
		return err
	}
	_, err = w.tx.Exec(
		"INSERT OR REPLACE INTO content_graph_properties (graph_id, key, value) VALUES (?, ?, ?)",
		graphID, key, string(data),
	)
	if err != nil {
		return fmt.Errorf("setting graph property %q on path %v: %w", key, path, err)
	}
	return nil
}

// DeleteCatalogContent deletes all content rows for a catalog name.
// ON DELETE CASCADE on child tables handles cleanup automatically.
func DeleteCatalogContent(tx *sql.Tx, catalogName string) error {
	for _, stmt := range []string{
		"DELETE FROM content_graphs WHERE catalog_name = ?",
		"DELETE FROM content_bundles WHERE catalog_name = ?",
	} {
		if _, err := tx.Exec(stmt, catalogName); err != nil {
			return fmt.Errorf("deleting catalog content for %q: %w", catalogName, err)
		}
	}
	return nil
}
