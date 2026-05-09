package catalogv1

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"maps"

	"k8s.io/apimachinery/pkg/labels"
	_ "modernc.org/sqlite"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/v1/internal"
)

type db struct {
	sqlDB *sql.DB
}

// OpenStore opens (or creates) a SQLite-backed Store at the given path.
func OpenStore(path string) (Store, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;
		PRAGMA foreign_keys=ON;
	`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("setting pragmas: %w", err)
	}

	// Run metadata migrations
	if err := internal.RunMetadataMigrations(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("running metadata migrations: %w", err)
	}

	// Check content schema version
	ok, err := internal.CheckContentSchemaVersion(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("checking content schema version: %w", err)
	}
	if !ok {
		// Either no content tables exist or version mismatch — rebuild
		if err := internal.DropContentTables(sqlDB); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("dropping content tables: %w", err)
		}
		if err := internal.CreateContentTables(sqlDB); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("creating content tables: %w", err)
		}
		if err := internal.StoreContentSchemaVersion(sqlDB); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("storing content schema version: %w", err)
		}
		if err := internal.ClearAllDigests(sqlDB); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("clearing digests: %w", err)
		}
	}

	return &db{sqlDB: sqlDB}, nil
}

func (d *db) Set(ctx context.Context, name string, opts ...SetOption) (Catalog, error) {
	var cfg setConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Check if the catalog entry already exists
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM catalog_metadata WHERE name = ?", name).Scan(&exists)
	isNew := err == sql.ErrNoRows
	if err != nil && !isNew {
		return nil, fmt.Errorf("checking catalog existence: %w", err)
	}

	if isNew {
		// For new entries, URI is required
		if cfg.uri == nil {
			return nil, fmt.Errorf("WithURI is required when creating a new catalog entry")
		}

		uri := *cfg.uri
		priority := 0
		if cfg.priority != nil {
			priority = *cfg.priority
		}

		_, err := tx.ExecContext(ctx,
			"INSERT INTO catalog_metadata (name, uri, priority) VALUES (?, ?, ?)",
			name, uri, priority,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting catalog metadata: %w", err)
		}
	} else {
		// Update only specified fields
		if cfg.uri != nil {
			if _, err := tx.ExecContext(ctx,
				"UPDATE catalog_metadata SET uri = ? WHERE name = ?",
				*cfg.uri, name,
			); err != nil {
				return nil, fmt.Errorf("updating uri: %w", err)
			}
		}
		if cfg.priority != nil {
			if _, err := tx.ExecContext(ctx,
				"UPDATE catalog_metadata SET priority = ? WHERE name = ?",
				*cfg.priority, name,
			); err != nil {
				return nil, fmt.Errorf("updating priority: %w", err)
			}
		}
	}

	// Handle content import
	var importErr error
	if cfg.content != nil {
		if err := internal.DeleteCatalogContent(tx, name); err != nil {
			return nil, fmt.Errorf("deleting existing content: %w", err)
		}

		w := &writerAdapter{
			cw: internal.NewContentWriter(tx, name),
		}
		if err := cfg.content.importer.Import(ctx, w); err != nil {
			if _, ok := err.(PartialImportError); !ok {
				return nil, fmt.Errorf("importing content: %w", err)
			}
			importErr = err
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE catalog_metadata SET digest = ? WHERE name = ?",
			cfg.content.digest, name,
		); err != nil {
			return nil, fmt.Errorf("updating digest: %w", err)
		}
	}

	// Handle labels
	if cfg.labels != nil {
		if v, ok := (*cfg.labels)[labelCatalogName]; ok && v != name {
			return nil, fmt.Errorf("label %q is reserved and must match the catalog name", labelCatalogName)
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM catalog_labels WHERE catalog_name = ?", name,
		); err != nil {
			return nil, fmt.Errorf("deleting existing labels: %w", err)
		}
		for k, v := range *cfg.labels {
			if k == labelCatalogName {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO catalog_labels (catalog_name, key, value) VALUES (?, ?, ?)",
				name, k, v,
			); err != nil {
				return nil, fmt.Errorf("inserting label %q: %w", k, err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT OR REPLACE INTO catalog_labels (catalog_name, key, value) VALUES (?, ?, ?)",
		name, labelCatalogName, name,
	); err != nil {
		return nil, fmt.Errorf("inserting reserved label: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}
	cat, err := d.Get(name)
	if err != nil {
		return nil, err
	}
	return cat, importErr
}

func (d *db) Get(name string) (Catalog, error) {
	var uri, digest string
	var priority int
	err := d.sqlDB.QueryRow(
		"SELECT uri, digest, priority FROM catalog_metadata WHERE name = ?", name,
	).Scan(&uri, &digest, &priority)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("catalog %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying catalog metadata: %w", err)
	}

	labels, err := d.queryLabels(name)
	if err != nil {
		return nil, err
	}

	return &storedCatalog{
		name:     name,
		uri:      uri,
		digest:   digest,
		priority: priority,
		labels:   labels,
		query:    &internal.CatalogQuery{DB: d.sqlDB, CatalogName: name},
	}, nil
}

func (d *db) Delete(name string) error {
	tx, err := d.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM catalog_metadata WHERE name = ?", name); err != nil {
		return fmt.Errorf("deleting catalog: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (d *db) List() ([]Catalog, error) {
	rows, err := d.sqlDB.Query("SELECT name, uri, digest, priority FROM catalog_metadata ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("querying catalogs: %w", err)
	}

	// Collect all metadata rows first, then close the cursor before
	// querying labels. With MaxOpenConns(1), holding the cursor open
	// while issuing another query would deadlock.
	type catalogMeta struct {
		name, uri, digest string
		priority          int
	}
	var metas []catalogMeta
	for rows.Next() {
		var m catalogMeta
		if err := rows.Scan(&m.name, &m.uri, &m.digest, &m.priority); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning catalog row: %w", err)
		}
		metas = append(metas, m)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating catalog rows: %w", err)
	}
	_ = rows.Close()

	catalogs := make([]Catalog, 0, len(metas))
	for _, m := range metas {
		labels, err := d.queryLabels(m.name)
		if err != nil {
			return nil, err
		}

		catalogs = append(catalogs, &storedCatalog{
			name:     m.name,
			uri:      m.uri,
			digest:   m.digest,
			priority: m.priority,
			labels:   labels,
			query:    &internal.CatalogQuery{DB: d.sqlDB, CatalogName: m.name},
		})
	}
	return catalogs, nil
}

func (d *db) Close() error {
	return d.sqlDB.Close()
}

func (d *db) queryLabels(catalogName string) (map[string]string, error) {
	rows, err := d.sqlDB.Query(
		"SELECT key, value FROM catalog_labels WHERE catalog_name = ?", catalogName,
	)
	if err != nil {
		return nil, fmt.Errorf("querying labels for %q: %w", catalogName, err)
	}
	defer func() { _ = rows.Close() }()

	labels := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning label row: %w", err)
		}
		labels[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating label rows: %w", err)
	}
	return labels, nil
}

// storedCatalog implements Catalog backed by metadata and a CatalogQuery.
type storedCatalog struct {
	name     string
	uri      string
	digest   string
	priority int
	labels   map[string]string
	query    *internal.CatalogQuery
}

func (c *storedCatalog) Name() string              { return c.name }
func (c *storedCatalog) URI() string               { return c.uri }
func (c *storedCatalog) Digest() string            { return c.digest }
func (c *storedCatalog) Priority() int             { return c.priority }
func (c *storedCatalog) Labels() map[string]string { return maps.Clone(c.labels) }

func (c *storedCatalog) ListPackages(ctx context.Context) iter.Seq2[UpdateGraph, error] {
	return func(yield func(UpdateGraph, error) bool) {
		for pkg, err := range c.query.ListPackages(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(&compositeUpdateGraphWrapper{q: pkg}, nil) {
				return
			}
		}
	}
}

func (c *storedCatalog) GetPackage(ctx context.Context, name string) (UpdateGraph, error) {
	q, err := c.query.GetPackage(ctx, name)
	if err != nil {
		return nil, err
	}
	return &compositeUpdateGraphWrapper{q: q}, nil
}

// compositeUpdateGraphWrapper wraps an internal CompositeUpdateGraphQuery to
// satisfy the CompositeUpdateGraph interface, converting internal concrete
// return types to the interface types.
type compositeUpdateGraphWrapper struct {
	q *internal.CompositeUpdateGraphQuery
}

func (w *compositeUpdateGraphWrapper) Name() string { return w.q.Name() }

func (w *compositeUpdateGraphWrapper) Property(ctx context.Context, key string) (json.RawMessage, error) {
	return w.q.Property(ctx, key)
}

func (w *compositeUpdateGraphWrapper) ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return w.q.ListBundles(ctx)
}

func (w *compositeUpdateGraphWrapper) Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	return w.q.Successors(ctx, from)
}

func (w *compositeUpdateGraphWrapper) ListGraphs(ctx context.Context) iter.Seq2[UpdateGraph, error] {
	return func(yield func(UpdateGraph, error) bool) {
		for g, err := range w.q.ListGraphs(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(g, nil) {
				return
			}
		}
	}
}

func (w *compositeUpdateGraphWrapper) GetGraph(ctx context.Context, name string) (UpdateGraph, error) {
	id, hasChildren, err := w.q.GetGraph(ctx, name)
	if err != nil {
		return nil, err
	}
	if hasChildren {
		return &compositeUpdateGraphWrapper{
			q: &internal.CompositeUpdateGraphQuery{DB: w.q.DB, CatalogName: w.q.CatalogName, GraphID: id, GraphName: name},
		}, nil
	}
	return &internal.UpdateGraphQuery{DB: w.q.DB, CatalogName: w.q.CatalogName, GraphID: id, GraphName: name}, nil
}

func (d *db) Select(selector labels.Selector) StoreReader {
	return &selectedStore{db: d, selector: selector}
}

// selectedStore is a read-only view of a db filtered by label selector.
type selectedStore struct {
	db       *db
	selector labels.Selector
}

func (s *selectedStore) Get(name string) (Catalog, error) {
	cat, err := s.db.Get(name)
	if err != nil {
		return nil, err
	}
	if !s.selector.Matches(labels.Set(cat.Labels())) {
		return nil, fmt.Errorf("catalog %q not found", name)
	}
	return cat, nil
}

func (s *selectedStore) List() ([]Catalog, error) {
	all, err := s.db.List()
	if err != nil {
		return nil, err
	}
	var filtered []Catalog
	for _, cat := range all {
		if s.selector.Matches(labels.Set(cat.Labels())) {
			filtered = append(filtered, cat)
		}
	}
	return filtered, nil
}

func (s *selectedStore) Select(selector labels.Selector) StoreReader {
	return &selectedStore{db: s.db, selector: andSelector(s.selector, selector)}
}

func andSelector(a, b labels.Selector) labels.Selector {
	reqs, _ := b.Requirements()
	return a.Add(reqs...)
}

// writerAdapter adapts the internal ContentWriter to the catalogv1.Writer interface.
type writerAdapter struct {
	cw *internal.ContentWriter
}

func (w *writerAdapter) InsertBundle(id, pkg, version, release, uri string) error {
	return w.cw.InsertBundle(id, pkg, version, release, uri)
}

func (w *writerAdapter) CreateGraph(path []string) error {
	return w.cw.CreateGraph(path)
}

func (w *writerAdapter) AddBundleToGraph(path []string, bundleID string) error {
	return w.cw.AddBundleToGraph(path, bundleID)
}

func (w *writerAdapter) AddEdge(path []string, fromBundleID, toBundleID string) error {
	return w.cw.AddEdge(path, fromBundleID, toBundleID)
}

func (w *writerAdapter) AddPredecessorRange(path []string, bundleID, versionRange string) error {
	return w.cw.AddPredecessorRange(path, bundleID, versionRange)
}

func (w *writerAdapter) SetBundleProperty(bundleID, key string, val any) error {
	return w.cw.SetBundleProperty(bundleID, key, val)
}

func (w *writerAdapter) SetGraphProperty(path []string, key string, val any) error {
	return w.cw.SetGraphProperty(path, key, val)
}

// Compile-time interface checks.
var _ Store = (*db)(nil)
var _ StoreReader = (*selectedStore)(nil)
var _ Catalog = (*storedCatalog)(nil)
var _ CompositeUpdateGraph = (*compositeUpdateGraphWrapper)(nil)
var _ UpdateGraph = (*internal.UpdateGraphQuery)(nil)
var _ Writer = (*writerAdapter)(nil)
var _ bundlev1.Bundle = internal.BundleRow{}
