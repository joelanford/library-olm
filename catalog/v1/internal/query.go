package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
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

// BundleRow implements bundlev1.Bundle backed by a row from the content_bundles table.
type BundleRow struct {
	DB          *sql.DB
	CatalogName string
	BundleID    bundlev1.BundleID
	PackageName string
	Version     bsemver.Version
	Release     bundlev1.Release
	BundleURI   string
}

func (b BundleRow) ID() bundlev1.BundleID { return b.BundleID }
func (b BundleRow) NameVersionRelease() bundlev1.NameVersionRelease {
	return bundlev1.NameVersionRelease{Name: b.PackageName, Version: b.Version, Release: b.Release}
}
func (b BundleRow) URI() string { return b.BundleURI }

func (b BundleRow) Property(ctx context.Context, key string) (json.RawMessage, error) {
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

// CatalogQuery provides catalog query operations scoped to a single catalog
// within the shared content tables.
type CatalogQuery struct {
	DB          *sql.DB
	CatalogName string
}

// ListPackages returns an iterator over the top-level composite update graphs
// (packages) for this catalog.
func (c *CatalogQuery) ListPackages(ctx context.Context) iter.Seq2[*CompositeUpdateGraphQuery, error] {
	return func(yield func(*CompositeUpdateGraphQuery, error) bool) {
		results, err := collectCompositeUpdateGraphResults(ctx, c.DB, c.CatalogName,
			"SELECT id, name FROM content_graphs WHERE parent_id IS NULL AND catalog_name = ? ORDER BY name",
			c.CatalogName)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, result := range results {
			if !yield(result.graph, result.err) {
				return
			}
		}
	}
}

// GetPackage returns the composite update graph for a specific package in this catalog.
func (c *CatalogQuery) GetPackage(ctx context.Context, name string) (*CompositeUpdateGraphQuery, error) {
	var id int64
	err := c.DB.QueryRowContext(ctx,
		"SELECT id FROM content_graphs WHERE name = ? AND parent_id IS NULL AND catalog_name = ?",
		name, c.CatalogName,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("package %q not found", name)
	}
	if err != nil {
		return nil, err
	}
	return &CompositeUpdateGraphQuery{DB: c.DB, CatalogName: c.CatalogName, GraphID: id, GraphName: name}, nil
}

// CompositeUpdateGraphQuery provides query operations for a composite update graph
// (a package with child graphs like channels).
type CompositeUpdateGraphQuery struct {
	DB          *sql.DB
	CatalogName string
	GraphID     int64
	GraphName   string
}

func (g *CompositeUpdateGraphQuery) Name() string { return g.GraphName }

func (g *CompositeUpdateGraphQuery) Property(ctx context.Context, key string) (json.RawMessage, error) {
	return queryGraphProperty(ctx, g.DB, g.GraphID, key)
}

func (g *CompositeUpdateGraphQuery) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return queryBundlesDescendant(ctx, g.DB, g.CatalogName, g.GraphID)
}

func (g *CompositeUpdateGraphQuery) Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDescendant(ctx, g.DB, g.CatalogName, g.GraphID, from.ID(), from.NameVersionRelease().Version)
}

// ListGraphs returns an iterator over the child update graphs.
func (g *CompositeUpdateGraphQuery) ListGraphs(ctx context.Context) iter.Seq2[*UpdateGraphQuery, error] {
	return func(yield func(*UpdateGraphQuery, error) bool) {
		results, err := collectUpdateGraphResults(ctx, g.DB, g.CatalogName,
			"SELECT id, name FROM content_graphs WHERE parent_id = ? ORDER BY name",
			g.GraphID)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, result := range results {
			if !yield(result.graph, result.err) {
				return
			}
		}
	}
}

// GetGraph returns a specific child update graph by name and whether it has children of its own.
func (g *CompositeUpdateGraphQuery) GetGraph(ctx context.Context, name string) (id int64, hasChildren bool, err error) {
	err = g.DB.QueryRowContext(ctx,
		`SELECT g.id, EXISTS(SELECT 1 FROM content_graphs c WHERE c.parent_id = g.id)
		 FROM content_graphs g
		 WHERE g.name = ? AND g.parent_id = ?`, name, g.GraphID,
	).Scan(&id, &hasChildren)
	if err == sql.ErrNoRows {
		return 0, false, fmt.Errorf("graph %q not found in %q", name, g.GraphName)
	}
	return id, hasChildren, err
}

// UpdateGraphQuery provides query operations for a leaf update graph (e.g. a channel).
type UpdateGraphQuery struct {
	DB          *sql.DB
	CatalogName string
	GraphID     int64
	GraphName   string
}

func (g *UpdateGraphQuery) Name() string { return g.GraphName }

func (g *UpdateGraphQuery) Property(ctx context.Context, key string) (json.RawMessage, error) {
	return queryGraphProperty(ctx, g.DB, g.GraphID, key)
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

func (g *UpdateGraphQuery) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return queryBundlesDirect(ctx, g.DB, g.CatalogName, g.GraphID)
}

func (g *UpdateGraphQuery) Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDirect(ctx, g.DB, g.CatalogName, g.GraphID, from.ID(), from.NameVersionRelease().Version)
}

func queryBundlesDirect(ctx context.Context, db *sql.DB, catalogName string, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		results, err := collectBundleResults(ctx, db, catalogName, `
			SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri
			FROM content_graph_bundles gb
			JOIN content_bundles b ON b.id = gb.bundle_id
			WHERE gb.graph_id = ? AND b.version != ''
			ORDER BY b.bundle_id`, graphID)
		if err != nil {
			yield(nil, err)
			return
		}
		yieldBundleResults(results, yield)
	}
}

func queryBundlesDescendant(ctx context.Context, db *sql.DB, catalogName string, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		results, err := collectBundleResults(ctx, db, catalogName, `
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
		if err != nil {
			yield(nil, err)
			return
		}
		yieldBundleResults(results, yield)
	}
}

func querySuccessorsDirect(ctx context.Context, db *sql.DB, catalogName string, graphID int64, fromID bundlev1.BundleID, fromVersion bsemver.Version) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsCollected(ctx, db, catalogName,
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
	return querySuccessorsCollected(ctx, db, catalogName,
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

func querySuccessorsCollected(
	ctx context.Context, db *sql.DB, catalogName string,
	explicitSQL string, explicitArgs []any,
	rangeSQL string, rangeArgs []any,
	version bsemver.Version,
) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		results, err := collectSuccessorResults(ctx, db, catalogName, explicitSQL, explicitArgs, rangeSQL, rangeArgs, version)
		if err != nil {
			yield(nil, err)
			return
		}
		yieldBundleResults(results, yield)
	}
}

func collectSuccessorResults(
	ctx context.Context, db *sql.DB, catalogName string,
	explicitSQL string, explicitArgs []any,
	rangeSQL string, rangeArgs []any,
	version bsemver.Version,
) ([]bundleResult, error) {
	seen := make(map[string]bool)
	explicitResults, err := collectExplicitSuccessorResults(ctx, db, catalogName, explicitSQL, explicitArgs, seen)
	if err != nil {
		return nil, err
	}
	rangeResults, err := collectRangeSuccessorResults(ctx, db, catalogName, rangeSQL, rangeArgs, version, seen)
	if err != nil {
		return nil, err
	}
	return append(explicitResults, rangeResults...), nil
}

func collectExplicitSuccessorResults(ctx context.Context, db *sql.DB, catalogName string, query string, args []any, seen map[string]bool) ([]bundleResult, error) {
	var results []bundleResult
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, packageName, versionStr, releaseStr, uri string
		if err := rows.Scan(&id, &packageName, &versionStr, &releaseStr, &uri); err != nil {
			results = append(results, bundleResult{err: err})
			continue
		}
		seen[id] = true
		b, err := parseBundleRow(db, catalogName, id, packageName, versionStr, releaseStr, uri)
		results = append(results, bundleResult{bundle: b, err: err})
	}
	if err := rows.Err(); err != nil {
		results = append(results, bundleResult{err: err})
	}
	return results, nil
}

func collectRangeSuccessorResults(ctx context.Context, db *sql.DB, catalogName string, query string, args []any, version bsemver.Version, seen map[string]bool) ([]bundleResult, error) {
	var results []bundleResult
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var bid, packageName, versionStr, releaseStr, uri, rangeStr string
		if err := rows.Scan(&bid, &packageName, &versionStr, &releaseStr, &uri, &rangeStr); err != nil {
			results = append(results, bundleResult{err: err})
			continue
		}
		if seen[bid] {
			continue
		}
		rng, err := bsemver.ParseRange(rangeStr)
		if err != nil {
			results = append(results, bundleResult{err: fmt.Errorf("parse predecessor range %q: %w", rangeStr, err)})
			continue
		}
		if !rng(version) {
			continue
		}
		seen[bid] = true
		b, err := parseBundleRow(db, catalogName, bid, packageName, versionStr, releaseStr, uri)
		results = append(results, bundleResult{bundle: b, err: err})
	}
	if err := rows.Err(); err != nil {
		results = append(results, bundleResult{err: err})
	}
	return results, nil
}

type compositeUpdateGraphResult struct {
	graph *CompositeUpdateGraphQuery
	err   error
}

func collectCompositeUpdateGraphResults(ctx context.Context, db *sql.DB, catalogName, query string, args ...any) ([]compositeUpdateGraphResult, error) {
	var results []compositeUpdateGraphResult
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			results = append(results, compositeUpdateGraphResult{err: err})
			continue
		}
		results = append(results, compositeUpdateGraphResult{
			graph: &CompositeUpdateGraphQuery{DB: db, CatalogName: catalogName, GraphID: id, GraphName: name},
		})
	}
	if err := rows.Err(); err != nil {
		results = append(results, compositeUpdateGraphResult{err: err})
	}
	return results, nil
}

type updateGraphResult struct {
	graph *UpdateGraphQuery
	err   error
}

func collectUpdateGraphResults(ctx context.Context, db *sql.DB, catalogName, query string, args ...any) ([]updateGraphResult, error) {
	var results []updateGraphResult
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			results = append(results, updateGraphResult{err: err})
			continue
		}
		results = append(results, updateGraphResult{
			graph: &UpdateGraphQuery{DB: db, CatalogName: catalogName, GraphID: id, GraphName: name},
		})
	}
	if err := rows.Err(); err != nil {
		results = append(results, updateGraphResult{err: err})
	}
	return results, nil
}

type bundleResult struct {
	bundle bundlev1.Bundle
	err    error
}

func collectBundleResults(ctx context.Context, db *sql.DB, catalogName, query string, args ...any) ([]bundleResult, error) {
	var results []bundleResult
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, packageName, versionStr, releaseStr, uri string
		if err := rows.Scan(&id, &packageName, &versionStr, &releaseStr, &uri); err != nil {
			results = append(results, bundleResult{err: err})
			continue
		}
		b, err := parseBundleRow(db, catalogName, id, packageName, versionStr, releaseStr, uri)
		results = append(results, bundleResult{bundle: b, err: err})
	}
	if err := rows.Err(); err != nil {
		results = append(results, bundleResult{err: err})
	}
	return results, nil
}

func yieldBundleResults(results []bundleResult, yield func(bundlev1.Bundle, error) bool) {
	for _, result := range results {
		if !yield(result.bundle, result.err) {
			return
		}
	}
}

func parseBundleRow(db *sql.DB, catalogName, id, packageName, versionStr, releaseStr, uri string) (BundleRow, error) {
	var ver bsemver.Version
	if versionStr != "" {
		parsed, err := bsemver.Parse(versionStr)
		if err != nil {
			return BundleRow{}, fmt.Errorf("parse version %q: %w", versionStr, err)
		}
		ver = parsed
	}
	return BundleRow{
		DB:          db,
		CatalogName: catalogName,
		BundleID:    bundlev1.BundleID(id),
		PackageName: packageName,
		Version:     ver,
		Release:     bundlev1.MustParseRelease(releaseStr),
		BundleURI:   uri,
	}, nil
}
