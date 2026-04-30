package internal

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"github.com/blang/semver/v4"
	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

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

func (g *CompositeUpdateGraphQuery) Successors(ctx context.Context, from bundlev1.Bundle) iter.Seq2[bundlev1.Bundle, error] {
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

func (g *UpdateGraphQuery) Successors(ctx context.Context, from bundlev1.Bundle) iter.Seq2[bundlev1.Bundle, error] {
	return querySuccessorsDirect(ctx, g.DB, g.GraphID, from)
}

func queryBundlesDirect(ctx context.Context, db *sql.DB, graphID int64) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			SELECT b.id, b.version, b.release
			FROM graph_bundles gb
			JOIN bundles b ON b.id = gb.bundle_id
			WHERE gb.graph_id = ? AND b.version != ''
			ORDER BY b.id`, graphID)
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
			SELECT DISTINCT b.id, b.version, b.release
			FROM graph_bundles gb
			JOIN bundles b ON b.id = gb.bundle_id
			WHERE gb.graph_id IN (SELECT id FROM descendants) AND b.version != ''
			ORDER BY b.id`, graphID)
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		yieldBundleRows(rows, yield)
	}
}

func querySuccessorsDirect(ctx context.Context, db *sql.DB, graphID int64, from bundlev1.Bundle) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			SELECT b.id, b.version, b.release
			FROM successors s
			JOIN bundles b ON b.id = s.to_bundle_id
			WHERE s.graph_id = ? AND s.from_bundle_id = ?
			ORDER BY b.id`, graphID, from.Name())
		if err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = rows.Close() }()
		yieldBundleRows(rows, yield)
	}
}

func querySuccessorsDescendant(ctx context.Context, db *sql.DB, graphID int64, from bundlev1.Bundle) iter.Seq2[bundlev1.Bundle, error] {
	return func(yield func(bundlev1.Bundle, error) bool) {
		rows, err := db.QueryContext(ctx, `
			WITH RECURSIVE descendants(id) AS (
				SELECT id FROM graphs WHERE parent_id = ?
				UNION ALL
				SELECT g.id FROM graphs g JOIN descendants d ON g.parent_id = d.id
			)
			SELECT DISTINCT b.id, b.version, b.release
			FROM successors s
			JOIN bundles b ON b.id = s.to_bundle_id
			WHERE s.graph_id IN (SELECT id FROM descendants) AND s.from_bundle_id = ?
			ORDER BY b.id`, graphID, from.Name())
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
		var name, versionStr, releaseStr string
		if err := rows.Scan(&name, &versionStr, &releaseStr); err != nil {
			if !yield(nil, err) {
				return
			}
			continue
		}
		var ver semver.Version
		if versionStr != "" {
			var err error
			ver, err = semver.Parse(versionStr)
			if err != nil {
				if !yield(nil, fmt.Errorf("parse version %q: %w", versionStr, err)) {
					return
				}
				continue
			}
		}
		rel := bundlev1.MustParseRelease(releaseStr)
		b := bundlev1.NameVersionRelease{BundleName: name, Version: ver, Release: rel}
		if !yield(b, nil) {
			return
		}
	}
	if err := rows.Err(); err != nil {
		yield(nil, err)
	}
}
