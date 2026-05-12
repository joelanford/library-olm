package sqlite

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

type contentWriter struct {
	tx          *sql.Tx
	catalogName string
}

func newContentWriter(tx *sql.Tx, catalogName string) *contentWriter {
	return &contentWriter{tx: tx, catalogName: catalogName}
}

func (w *contentWriter) deleteAll() error {
	for _, stmt := range []string{
		"DELETE FROM content_graphs WHERE catalog_name = ?",
		"DELETE FROM content_bundles WHERE catalog_name = ?",
	} {
		if _, err := w.tx.Exec(stmt, w.catalogName); err != nil {
			return fmt.Errorf("deleting catalog content for %q: %w", w.catalogName, err)
		}
	}
	return nil
}

func (w *contentWriter) InsertBundle(id, pkg, version, release, uri string) error {
	_, err := w.tx.Exec(
		"INSERT OR IGNORE INTO content_bundles (catalog_name, bundle_id, package_name, version, release, uri) VALUES (?, ?, ?, ?, ?, ?)",
		w.catalogName, id, pkg, version, release, uri,
	)
	if err != nil {
		return fmt.Errorf("inserting bundle %q: %w", id, err)
	}
	return nil
}

func (w *contentWriter) resolveGraphID(path []string) (int64, error) {
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

func (w *contentWriter) CreateGraph(path []string) error {
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

func (w *contentWriter) AddBundleToGraph(path []string, bundleID string) error {
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

func (w *contentWriter) AddEdge(path []string, fromBundleID, toBundleID string) error {
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

func (w *contentWriter) AddPredecessorRange(path []string, bundleID, versionRange string) error {
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

func (w *contentWriter) SetBundleProperty(bundleID, key string, val any) error {
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

func (w *contentWriter) SetGraphProperty(path []string, key string, val any) error {
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

func (w *contentWriter) SetGraphDeprecation(path []string, message string) error {
	p, err := graphPath(path)
	if err != nil {
		return err
	}
	_, err = w.tx.Exec(
		"UPDATE content_graphs SET deprecation_message = ? WHERE catalog_name = ? AND path = ?",
		message, w.catalogName, p,
	)
	if err != nil {
		return fmt.Errorf("setting graph deprecation on path %v: %w", path, err)
	}
	return nil
}

func (w *contentWriter) SetBundleDeprecation(bundleID string, message string) error {
	_, err := w.tx.Exec(
		"UPDATE content_bundles SET deprecation_message = ? WHERE catalog_name = ? AND bundle_id = ?",
		message, w.catalogName, bundleID,
	)
	if err != nil {
		return fmt.Errorf("setting bundle deprecation on %q: %w", bundleID, err)
	}
	return nil
}
