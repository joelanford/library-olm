package internal

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// BundleRow implements bundlev1.Bundle backed by a row from the normalized bundles table.
type BundleRow struct {
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

// CatalogQuery implements catalogv1.Catalog backed by normalized SQLite tables.
type CatalogQuery struct {
	DB *sql.DB
}

func (c *CatalogQuery) ListPackages(ctx context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	return func(yield func(catalogv1.UpdateGraph, error) bool) {
		rows, err := c.DB.QueryContext(ctx, "SELECT id, name FROM graphs WHERE parent_id IS NULL ORDER BY name")
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(&CompositeUpdateGraphQuery{DB: c.DB, GraphID: id, GraphName: name}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func (c *CatalogQuery) GetPackage(ctx context.Context, name string) (catalogv1.UpdateGraph, error) {
	var id int64
	err := c.DB.QueryRowContext(ctx, "SELECT id FROM graphs WHERE name = ? AND parent_id IS NULL", name).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("package %q not found", name)
	}
	if err != nil {
		return nil, err
	}
	return &CompositeUpdateGraphQuery{DB: c.DB, GraphID: id, GraphName: name}, nil
}

// CompositeUpdateGraphQuery implements catalogv1.CompositeUpdateGraph.
type CompositeUpdateGraphQuery struct {
	DB        *sql.DB
	GraphID   int64
	GraphName string
}

func (g *CompositeUpdateGraphQuery) Name() string { return g.GraphName }

func (g *CompositeUpdateGraphQuery) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return queryBundlesDescendant(ctx, g.DB, g.GraphID)
}

func (g *CompositeUpdateGraphQuery) Successors(ctx context.Context, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDescendant(ctx, g.DB, g.GraphID, from)
}

func (g *CompositeUpdateGraphQuery) ListGraphs(ctx context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	return func(yield func(catalogv1.UpdateGraph, error) bool) {
		rows, err := g.DB.QueryContext(ctx, "SELECT id, name FROM graphs WHERE parent_id = ? ORDER BY name", g.GraphID)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id int64
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(&UpdateGraphQuery{DB: g.DB, GraphID: id, GraphName: name}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func (g *CompositeUpdateGraphQuery) GetGraph(ctx context.Context, name string) (catalogv1.UpdateGraph, error) {
	var id int64
	err := g.DB.QueryRowContext(ctx, "SELECT id FROM graphs WHERE name = ? AND parent_id = ?", name, g.GraphID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("graph %q not found in %q", name, g.GraphName)
	}
	if err != nil {
		return nil, err
	}
	return &UpdateGraphQuery{DB: g.DB, GraphID: id, GraphName: name}, nil
}

// UpdateGraphQuery implements catalogv1.UpdateGraph for a leaf graph (e.g. a channel).
type UpdateGraphQuery struct {
	DB        *sql.DB
	GraphID   int64
	GraphName string
}

func (g *UpdateGraphQuery) Name() string { return g.GraphName }

func (g *UpdateGraphQuery) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return queryBundlesDirect(ctx, g.DB, g.GraphID)
}

func (g *UpdateGraphQuery) Successors(ctx context.Context, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDirect(ctx, g.DB, g.GraphID, from)
}

func queryBundlesDirect(ctx context.Context, db *sql.DB, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri
			FROM graph_bundles gb
			JOIN bundles b ON b.id = gb.bundle_id
			WHERE gb.graph_id = ? AND b.version != ''
			ORDER BY b.bundle_id`, graphID)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		yieldBundleRows(rows, yield)
	}
}

func queryBundlesDescendant(ctx context.Context, db *sql.DB, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM graphs WHERE parent_id = ?
				UNION ALL
				SELECT g.id FROM graphs g JOIN descendants d ON g.parent_id = d.id
			)
			SELECT DISTINCT b.bundle_id, b.package_name, b.version, b.release, b.uri
			FROM graph_bundles gb
			JOIN bundles b ON b.id = gb.bundle_id
			WHERE gb.graph_id IN (SELECT id FROM descendants) AND b.version != ''
			ORDER BY b.bundle_id`, graphID)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		yieldBundleRows(rows, yield)
	}
}

func querySuccessorsDirect(ctx context.Context, db *sql.DB, graphID int64, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			SELECT b.bundle_id, b.package_name, b.version, b.release, b.uri
			FROM successors s
			JOIN bundles b ON b.id = s.to_bundle_id
			WHERE s.graph_id = ? AND s.from_bundle_id = (SELECT id FROM bundles WHERE bundle_id = ?)
			ORDER BY b.bundle_id`, graphID, string(from))
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		yieldBundleRows(rows, yield)
	}
}

func querySuccessorsDescendant(ctx context.Context, db *sql.DB, graphID int64, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM graphs WHERE parent_id = ?
				UNION ALL
				SELECT g.id FROM graphs g JOIN descendants d ON g.parent_id = d.id
			)
			SELECT DISTINCT b.bundle_id, b.package_name, b.version, b.release, b.uri
			FROM successors s
			JOIN bundles b ON b.id = s.to_bundle_id
			WHERE s.graph_id IN (SELECT id FROM descendants) AND s.from_bundle_id = (SELECT id FROM bundles WHERE bundle_id = ?)
			ORDER BY b.bundle_id`, graphID, string(from))
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		yieldBundleRows(rows, yield)
	}
}

func yieldBundleRows(rows *sql.Rows, yield func(bundlev1.Bundle, error) bool) {
	for rows.Next() {
		var id, packageName, versionStr, releaseStr, uri string
		if err := rows.Scan(&id, &packageName, &versionStr, &releaseStr, &uri); err != nil {
			if !yield(nil, err) {
				return
			}
			continue
		}
		var ver bsemver.Version
		if versionStr != "" {
			var err error
			ver, err = bsemver.Parse(versionStr)
			if err != nil {
				if !yield(nil, fmt.Errorf("parse version %q: %w", versionStr, err)) {
					return
				}
				continue
			}
		}
		rel := bundlev1.MustParseRelease(releaseStr)
		b := BundleRow{
			BundleID:    bundlev1.BundleID(id),
			PackageName: packageName,
			Version:     ver,
			Release:     rel,
			BundleURI:   uri,
		}
		if !yield(b, nil) {
			return
		}
	}
	if err := rows.Err(); err != nil {
		yield(nil, err)
	}
}
