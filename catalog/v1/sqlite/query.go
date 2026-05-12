package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"strings"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", name,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type bundleRow struct {
	DB          *sql.DB
	CatalogName string
	BundleID    bundlev1.BundleID
	PackageName string
	Version     bsemver.Version
	Release     bundlev1.Release
	BundleURI   string
}

func (b bundleRow) ID() bundlev1.BundleID { return b.BundleID }
func (b bundleRow) NameVersionRelease() bundlev1.NameVersionRelease {
	return bundlev1.NameVersionRelease{Name: b.PackageName, Version: b.Version, Release: b.Release}
}
func (b bundleRow) URI() string { return b.BundleURI }

func (b bundleRow) Property(ctx context.Context, key string) (json.RawMessage, error) {
	var val string
	err := b.DB.QueryRowContext(ctx,
		"SELECT value FROM content_bundle_properties WHERE catalog_name = ? AND bundle_id = ? AND key = ?",
		b.CatalogName, string(b.BundleID), key,
	).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying bundle property %q: %w", key, err)
	}
	return json.RawMessage(val), nil
}

// graphQuery implements catalogv1.UpdateGraph for leaf graphs.
type graphQuery struct {
	db          *sql.DB
	catalogName string
	graphID     int64
	graphName   string
}

func (g *graphQuery) Name() string { return g.graphName }

func (g *graphQuery) Property(ctx context.Context, key string) (json.RawMessage, error) {
	return queryGraphProperty(ctx, g.db, g.graphID, key)
}

func (g *graphQuery) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return queryBundlesDirect(ctx, g.db, g.catalogName, g.graphID)
}

func (g *graphQuery) Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDirect(ctx, g.db, g.catalogName, g.graphID, from.ID(), from.NameVersionRelease().Version)
}

// compositeGraphQuery implements catalogv1.CompositeUpdateGraph for composite graphs.
type compositeGraphQuery struct {
	graphQuery
	graphPath []string
}

func (g *compositeGraphQuery) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return queryBundlesDescendant(ctx, g.db, g.catalogName, g.graphID)
}

func (g *compositeGraphQuery) Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDescendant(ctx, g.db, g.catalogName, g.graphID, from.ID(), from.NameVersionRelease().Version)
}

func (g *compositeGraphQuery) ListGraphs(ctx context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	return queryGraphNodes(ctx, g.db, g.catalogName, &g.graphID, "", g.graphPath)
}

func (g *compositeGraphQuery) GetGraph(ctx context.Context, name string) (catalogv1.UpdateGraph, error) {
	return queryGraphNode(ctx, g.db, g.catalogName, &g.graphID, name, g.graphPath, fmt.Sprintf("graph %q not found in %s", name, strings.Join(g.graphPath, "/")))
}

func queryGraphProperty(ctx context.Context, db *sql.DB, graphID int64, key string) (json.RawMessage, error) {
	var val string
	err := db.QueryRowContext(ctx,
		"SELECT value FROM content_graph_properties WHERE graph_id = ? AND key = ?",
		graphID, key,
	).Scan(&val)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying graph property %q: %w", key, err)
	}
	return json.RawMessage(val), nil
}

func queryGraphNodes(ctx context.Context, db *sql.DB, catalogName string, parentID *int64, name string, parentPath []string) iter.Seq2[catalogv1.UpdateGraph, error] {
	where := `g.parent_id IS NULL AND g.catalog_name = ?`
	args := []any{catalogName}
	if parentID != nil {
		where = `g.parent_id = ?`
		args = []any{*parentID}
	}
	if name != "" {
		where += ` AND g.name = ?`
		args = append(args, name)
	}
	query := `SELECT g.id, g.name, EXISTS(SELECT 1 FROM content_graphs c WHERE c.parent_id = g.id)
		 FROM content_graphs g WHERE ` + where + ` ORDER BY g.name`

	return func(yield func(catalogv1.UpdateGraph, error) bool) {
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int64
			var nodeName string
			var hasChildren bool
			if err := rows.Scan(&id, &nodeName, &hasChildren); err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			path := append(slices.Clone(parentPath), nodeName)
			var ug catalogv1.UpdateGraph
			if hasChildren {
				ug = &compositeGraphQuery{
					graphQuery: graphQuery{db: db, catalogName: catalogName, graphID: id, graphName: nodeName},
					graphPath:  path,
				}
			} else {
				ug = &graphQuery{db: db, catalogName: catalogName, graphID: id, graphName: nodeName}
			}
			if !yield(ug, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func queryGraphNode(ctx context.Context, db *sql.DB, catalogName string, parentID *int64, name string, parentPath []string, notFoundMsg string) (catalogv1.UpdateGraph, error) {
	for ug, err := range queryGraphNodes(ctx, db, catalogName, parentID, name, parentPath) {
		return ug, err
	}
	return nil, fmt.Errorf("%s", notFoundMsg)
}

func queryBundlesDirect(ctx context.Context, db *sql.DB, catalogName string, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return streamBundleRows(ctx, db, catalogName, `
		SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri
		FROM content_graph_bundles gb
		JOIN content_bundles b ON b.id = gb.bundle_id
		WHERE gb.graph_id = ? AND b.version != ''
		ORDER BY b.bundle_id`, graphID)
}

func queryBundlesDescendant(ctx context.Context, db *sql.DB, catalogName string, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return streamBundleRows(ctx, db, catalogName, `
		WITH RECURSIVE descendants(id) AS (
			SELECT id FROM content_graphs WHERE parent_id = ?
			UNION ALL
			SELECT g.id FROM content_graphs g JOIN descendants d ON g.parent_id = d.id
		)
		SELECT DISTINCT b.bundle_id, b.package_name, b.version, b.release, b.uri
		FROM content_graph_bundles gb
		JOIN content_bundles b ON b.id = gb.bundle_id
		WHERE gb.graph_id IN (SELECT id FROM descendants) AND b.version != ''
		ORDER BY b.bundle_id`, graphID)
}

func streamBundleRows(ctx context.Context, db *sql.DB, catalogName, query string, args ...any) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id, packageName, versionStr, releaseStr, uri string
			if err := rows.Scan(&id, &packageName, &versionStr, &releaseStr, &uri); err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			b, err := parseBundleRow(db, catalogName, id, packageName, versionStr, releaseStr, uri)
			if !yield(b, err) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func querySuccessorsDirect(ctx context.Context, db *sql.DB, catalogName string, graphID int64, fromID bundlev1.BundleID, fromVersion bsemver.Version) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsStreaming(ctx, db, catalogName,
		`SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri
		 FROM content_successors s
		 JOIN content_bundles b ON b.id = s.to_bundle_id
		 WHERE s.graph_id = ? AND s.from_bundle_id = (SELECT id FROM content_bundles WHERE bundle_id = ?)`,
		[]any{graphID, string(fromID)},
		`SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri, pc.version_range
		 FROM content_predecessor_ranges pc
		 JOIN content_bundles b ON b.id = pc.bundle_id
		 WHERE pc.graph_id = ?`,
		[]any{graphID},
		fromVersion,
	)
}

func querySuccessorsDescendant(ctx context.Context, db *sql.DB, catalogName string, graphID int64, fromID bundlev1.BundleID, fromVersion bsemver.Version) iter.Seq2[bundlev1.Bundle, error] {
	const descendantCTE = `WITH RECURSIVE descendants(id) AS (
		SELECT id FROM content_graphs WHERE parent_id = ?
		UNION ALL
		SELECT g.id FROM content_graphs g JOIN descendants d ON g.parent_id = d.id
	)`
	return querySuccessorsStreaming(ctx, db, catalogName,
		descendantCTE+`
		SELECT DISTINCT b.bundle_id, b.package_name, b.version, b.release, b.uri
		FROM content_successors s
		JOIN content_bundles b ON b.id = s.to_bundle_id
		WHERE s.graph_id IN (SELECT id FROM descendants)
		  AND s.from_bundle_id = (SELECT id FROM content_bundles WHERE bundle_id = ?)`,
		[]any{graphID, string(fromID)},
		descendantCTE+`
		SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri, pc.version_range
		FROM content_predecessor_ranges pc
		JOIN content_bundles b ON b.id = pc.bundle_id
		WHERE pc.graph_id IN (SELECT id FROM descendants)`,
		[]any{graphID},
		fromVersion,
	)
}

func querySuccessorsStreaming(
	ctx context.Context, db *sql.DB, catalogName string,
	explicitSQL string, explicitArgs []any,
	rangeSQL string, rangeArgs []any,
	version bsemver.Version,
) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		seen := make(map[string]bool)

		rows, err := db.QueryContext(ctx, explicitSQL, explicitArgs...)
		if err != nil {
			yield(nil, err)
			return
		}
		for rows.Next() {
			var id, packageName, versionStr, releaseStr, uri string
			if err := rows.Scan(&id, &packageName, &versionStr, &releaseStr, &uri); err != nil {
				if !yield(nil, err) {
					_ = rows.Close()
					return
				}
				continue
			}
			seen[id] = true
			b, err := parseBundleRow(db, catalogName, id, packageName, versionStr, releaseStr, uri)
			if !yield(b, err) {
				_ = rows.Close()
				return
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			yield(nil, err)
			return
		}
		_ = rows.Close()

		rows, err = db.QueryContext(ctx, rangeSQL, rangeArgs...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var bid, packageName, versionStr, releaseStr, uri, rangeStr string
			if err := rows.Scan(&bid, &packageName, &versionStr, &releaseStr, &uri, &rangeStr); err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if seen[bid] {
				continue
			}
			rng, err := bsemver.ParseRange(rangeStr)
			if err != nil {
				if !yield(nil, fmt.Errorf("parse predecessor range %q: %w", rangeStr, err)) {
					return
				}
				continue
			}
			if !rng(version) {
				continue
			}
			seen[bid] = true
			b, err := parseBundleRow(db, catalogName, bid, packageName, versionStr, releaseStr, uri)
			if !yield(b, err) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func parseBundleRow(db *sql.DB, catalogName, id, packageName, versionStr, releaseStr, uri string) (bundleRow, error) {
	var ver bsemver.Version
	if versionStr != "" {
		parsed, err := bsemver.Parse(versionStr)
		if err != nil {
			return bundleRow{}, fmt.Errorf("parse version %q: %w", versionStr, err)
		}
		ver = parsed
	}
	return bundleRow{
		DB:          db,
		CatalogName: catalogName,
		BundleID:    bundlev1.BundleID(id),
		PackageName: packageName,
		Version:     ver,
		Release:     bundlev1.MustParseRelease(releaseStr),
		BundleURI:   uri,
	}, nil
}
