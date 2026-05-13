package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"maps"
	"net/url"

	"k8s.io/apimachinery/pkg/labels"
	_ "modernc.org/sqlite"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

const labelCatalogName = "olm.operatorframework.io/metadata.name"

type store struct {
	writerDB *sql.DB
	readerDB *sql.DB
}

// OpenStore opens (or creates) a SQLite-backed Store at the given path.
func OpenStore(path string) (catalogv1.Store, error) {
	writerDB, err := sql.Open("sqlite", sqliteDSN(path, false))
	if err != nil {
		return nil, fmt.Errorf("opening writer database: %w", err)
	}
	writerDB.SetMaxOpenConns(1)

	if err := runMetadataMigrations(writerDB); err != nil {
		_ = writerDB.Close()
		return nil, fmt.Errorf("running metadata migrations: %w", err)
	}

	ok, err := checkContentSchemaVersion(writerDB)
	if err != nil {
		_ = writerDB.Close()
		return nil, fmt.Errorf("checking content schema version: %w", err)
	}
	if !ok {
		if err := dropContentTables(writerDB); err != nil {
			_ = writerDB.Close()
			return nil, fmt.Errorf("dropping content tables: %w", err)
		}
		if err := createContentTables(writerDB); err != nil {
			_ = writerDB.Close()
			return nil, fmt.Errorf("creating content tables: %w", err)
		}
		if err := storeContentSchemaVersion(writerDB); err != nil {
			_ = writerDB.Close()
			return nil, fmt.Errorf("storing content schema version: %w", err)
		}
		if err := clearAllDigests(writerDB); err != nil {
			_ = writerDB.Close()
			return nil, fmt.Errorf("clearing digests: %w", err)
		}
	}

	readerDB, err := sql.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		_ = writerDB.Close()
		return nil, fmt.Errorf("opening reader database: %w", err)
	}

	return &store{writerDB: writerDB, readerDB: readerDB}, nil
}

func sqliteDSN(path string, readOnly bool) string {
	q := url.Values{}
	if readOnly {
		q.Set("mode", "ro")
	}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	return fmt.Sprintf("file:%s?%s", url.PathEscape(path), q.Encode())
}

func (d *store) Set(ctx context.Context, name string, opts ...catalogv1.SetOption) (catalogv1.Catalog, error) {
	cfg := catalogv1.ApplySetOptions(opts)

	tx, err := d.writerDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM catalog_metadata WHERE name = ?", name).Scan(&exists)
	isNew := err == sql.ErrNoRows
	if err != nil && !isNew {
		return nil, fmt.Errorf("checking catalog existence: %w", err)
	}

	if isNew {
		if cfg.URI == nil {
			return nil, fmt.Errorf("WithURI is required when creating a new catalog entry")
		}

		uri := *cfg.URI
		priority := 0
		if cfg.Priority != nil {
			priority = *cfg.Priority
		}

		_, err := tx.ExecContext(ctx,
			"INSERT INTO catalog_metadata (name, uri, priority) VALUES (?, ?, ?)",
			name, uri, priority,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting catalog metadata: %w", err)
		}
	} else {
		if cfg.URI != nil {
			if _, err := tx.ExecContext(ctx,
				"UPDATE catalog_metadata SET uri = ? WHERE name = ?",
				*cfg.URI, name,
			); err != nil {
				return nil, fmt.Errorf("updating uri: %w", err)
			}
		}
		if cfg.Priority != nil {
			if _, err := tx.ExecContext(ctx,
				"UPDATE catalog_metadata SET priority = ? WHERE name = ?",
				*cfg.Priority, name,
			); err != nil {
				return nil, fmt.Errorf("updating priority: %w", err)
			}
		}
	}

	var importErr error
	if cfg.Content != nil {
		w := newContentWriter(tx, name)
		if err := w.deleteAll(); err != nil {
			return nil, fmt.Errorf("deleting existing content: %w", err)
		}

		if err := cfg.Content.Importer.Import(ctx, w); err != nil {
			if _, ok := err.(catalogv1.PartialImportError); !ok {
				return nil, fmt.Errorf("importing content: %w", err)
			}
			importErr = err
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE catalog_metadata SET digest = ? WHERE name = ?",
			cfg.Content.Digest, name,
		); err != nil {
			return nil, fmt.Errorf("updating digest: %w", err)
		}
	}

	if cfg.Labels != nil {
		if v, ok := (*cfg.Labels)[labelCatalogName]; ok && v != name {
			return nil, fmt.Errorf("label %q is reserved and must match the catalog name", labelCatalogName)
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM catalog_labels WHERE catalog_name = ?", name,
		); err != nil {
			return nil, fmt.Errorf("deleting existing labels: %w", err)
		}
		for k, v := range *cfg.Labels {
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

	cat, err := getCatalog(tx, d.readerDB, name)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}
	return cat, importErr
}

func (d *store) Get(name string) (catalogv1.Catalog, error) {
	return getCatalog(d.readerDB, d.readerDB, name)
}

func (d *store) Delete(name string) error {
	tx, err := d.writerDB.Begin()
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

func (d *store) List() ([]catalogv1.Catalog, error) {
	rows, err := d.readerDB.Query("SELECT name, uri, digest, priority FROM catalog_metadata ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("querying catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var catalogs []catalogv1.Catalog
	for rows.Next() {
		var name, uri, digest string
		var priority int
		if err := rows.Scan(&name, &uri, &digest, &priority); err != nil {
			return nil, fmt.Errorf("scanning catalog row: %w", err)
		}

		lbls, err := queryLabels(d.readerDB, name)
		if err != nil {
			return nil, err
		}

		catalogs = append(catalogs, &storedCatalog{
			name:     name,
			uri:      uri,
			digest:   digest,
			priority: priority,
			labels:   lbls,
			readerDB: d.readerDB,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating catalog rows: %w", err)
	}
	return catalogs, nil
}

func (d *store) Close() error {
	readerErr := d.readerDB.Close()
	writerErr := d.writerDB.Close()
	if readerErr != nil {
		return readerErr
	}
	return writerErr
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func getCatalog(q querier, readerDB *sql.DB, name string) (*storedCatalog, error) {
	var uri, digest string
	var priority int
	err := q.QueryRow(
		"SELECT uri, digest, priority FROM catalog_metadata WHERE name = ?", name,
	).Scan(&uri, &digest, &priority)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("catalog %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying catalog metadata: %w", err)
	}

	lbls, err := queryLabels(q, name)
	if err != nil {
		return nil, err
	}

	return &storedCatalog{
		name:     name,
		uri:      uri,
		digest:   digest,
		priority: priority,
		labels:   lbls,
		readerDB: readerDB,
	}, nil
}

func queryLabels(q querier, catalogName string) (map[string]string, error) {
	rows, err := q.Query(
		"SELECT key, value FROM catalog_labels WHERE catalog_name = ?", catalogName,
	)
	if err != nil {
		return nil, fmt.Errorf("querying labels for %q: %w", catalogName, err)
	}
	defer func() { _ = rows.Close() }()

	lbls := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning label row: %w", err)
		}
		lbls[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating label rows: %w", err)
	}
	return lbls, nil
}

type storedCatalog struct {
	name     string
	uri      string
	digest   string
	priority int
	labels   map[string]string
	readerDB *sql.DB
}

func (c *storedCatalog) Name() string              { return c.name }
func (c *storedCatalog) URI() string               { return c.uri }
func (c *storedCatalog) Digest() string            { return c.digest }
func (c *storedCatalog) Priority() int             { return c.priority }
func (c *storedCatalog) Labels() map[string]string { return maps.Clone(c.labels) }

func (c *storedCatalog) ListPackages(ctx context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	return queryGraphNodes(ctx, c.readerDB, c.name, nil, "", nil)
}

func (c *storedCatalog) GetPackage(ctx context.Context, name string) (catalogv1.UpdateGraph, error) {
	return queryGraphNode(ctx, c.readerDB, c.name, nil, name, nil, fmt.Sprintf("package %q not found", name))
}

func (d *store) Select(selector labels.Selector) catalogv1.StoreReader {
	return &selectedStore{store: d, selector: selector}
}

type selectedStore struct {
	store    *store
	selector labels.Selector
}

func (s *selectedStore) Get(name string) (catalogv1.Catalog, error) {
	cat, err := s.store.Get(name)
	if err != nil {
		return nil, err
	}
	if !s.selector.Matches(labels.Set(cat.Labels())) {
		return nil, fmt.Errorf("catalog %q not found", name)
	}
	return cat, nil
}

func (s *selectedStore) List() ([]catalogv1.Catalog, error) {
	all, err := s.store.List()
	if err != nil {
		return nil, err
	}
	var filtered []catalogv1.Catalog
	for _, cat := range all {
		if s.selector.Matches(labels.Set(cat.Labels())) {
			filtered = append(filtered, cat)
		}
	}
	return filtered, nil
}

func (s *selectedStore) Select(selector labels.Selector) catalogv1.StoreReader {
	return &selectedStore{store: s.store, selector: andSelector(s.selector, selector)}
}

func andSelector(a, b labels.Selector) labels.Selector {
	reqs, _ := b.Requirements()
	return a.Add(reqs...)
}

// Compile-time interface checks.
var _ catalogv1.Store = (*store)(nil)
var _ catalogv1.StoreReader = (*selectedStore)(nil)
var _ catalogv1.Catalog = (*storedCatalog)(nil)
var _ catalogv1.CompositeUpdateGraph = (*compositeGraphQuery)(nil)
var _ catalogv1.UpdateGraph = (*graphQuery)(nil)
var _ catalogv1.Writer = (*contentWriter)(nil)
var _ bundlev1.Bundle = bundleRow{}
var _ catalogv1.Deprecated = deprecation{}
var _ catalogv1.Deprecated = (*deprecatedUpdateGraph)(nil)
var _ catalogv1.Deprecated = (*deprecatedCompositeUpdateGraph)(nil)
var _ catalogv1.Deprecated = deprecatedBundle{}
