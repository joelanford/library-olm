---
status: pr-submitted
pr: https://github.com/joelanford/library-olm/pull/2
---
# Persistent Catalog DB

## Summary

Refactor the catalog storage layer into a persistent, multi-catalog SQLite store with a two-layer schema: a **metadata layer** tracking catalog metadata (name, URI, digest, priority, labels) that uses standard database migrations, and a **content layer** storing normalized catalog data (bundles, graphs, successors) that is thrown away and rebuilt on schema mismatch. The store exposes a `catalogv1.Store` interface backed by an unexported SQLite implementation, with `Importer`/`Writer` interfaces for format-specific import logic. The existing `fbc.FromFS` is removed; FBC import becomes `fbc.NewImporter(fsys)` implementing `catalogv1.Importer`.

## Design

### Architecture split

The current design conflates format-specific logic (FBC parsing, handler dispatch) with format-agnostic concerns (normalized schema, querying, persistence). This spec separates them:

- **`catalog/v1/`** gains `Store`, `Catalog` (extended with metadata methods), `Importer`, `Writer`, and the normalized schema. The `Store` interface manages a persistent SQLite file with metadata and content layers. The unexported `db` type implements `Store`. Query types (`CatalogQuery`, `UpdateGraphQuery`, `CompositeUpdateGraphQuery`, `BundleRow`) move from `catalog/fbc/internal/` to `catalog/v1/internal/` since they operate on format-agnostic normalized tables.

- **`catalog/fbc/`** becomes a thin import layer. `fbc.NewImporter(fsys)` returns a `catalogv1.Importer` that manages its own temporary SQLite DB for staging, then writes through `Writer`. The existing `fbc.Catalog` type, `fbc.FromFS`, and `fbc.Close` are removed.

### Two-layer schema

**Metadata layer** — stable, migratable schema. Tracks which catalogs exist and their metadata. Uses standard database migrations so `OpenStore` succeeds even when the content schema is incompatible with the current library version. The metadata layer survives content rebuilds.

**Content layer** — normalized catalog data (bundles, graphs, successors). This is derived data rebuilt from source. On content schema mismatch, the content tables are dropped and recreated with the new schema. Metadata is preserved, and the caller re-imports using the URIs from the metadata layer.

On `OpenStore`:
1. Migrate the metadata schema forward
2. Check the content schema version
3. If mismatch, drop all non-metadata tables (query `sqlite_master`, keep only the known metadata tables: `catalog_metadata`, `catalog_labels`, `metadata_schema_version`) and recreate content tables with the new schema

### Catalog interface (`catalog/v1`)

The `Catalog` interface is extended with metadata methods, unifying metadata and content access into a single type:

```go
type Catalog interface {
    Name() string
    URI() string
    Digest() string
    Priority() int
    Labels() map[string]string

    ListPackages(ctx context.Context) iter.Seq2[UpdateGraph, error]
    GetPackage(ctx context.Context, name string) (UpdateGraph, error)
}
```

Catalogs returned by `Get` or `List` are snapshots: metadata (Name, URI, Digest, Priority, Labels) reflects the state at query time. Subsequent `Set` calls do not update previously returned `Catalog` values — call `Get` again for fresh metadata. Content queries (`ListPackages`, `GetPackage`) are lazy — they query the DB on demand. If content has not been imported yet, content queries return empty results.

### Store interface (`catalog/v1`)

```go
type Store interface {
    Set(ctx context.Context, name string, opts ...SetOption) (Catalog, error)
    Get(name string) (Catalog, error)
    Delete(name string) error
    List() ([]Catalog, error)
    Close() error
}

type SetOption func(*setConfig)

func WithURI(uri string) SetOption
func WithPriority(priority int) SetOption
func WithLabels(labels map[string]string) SetOption
func WithContent(importer Importer, digest string) SetOption

func OpenStore(path string) (Store, error)
```

`OpenStore` opens or creates the SQLite file. It migrates the metadata schema forward and checks the content schema version. If the content schema is incompatible, all non-metadata tables are dropped and recreated. The returned `Store` is an unexported `*db` type. A content schema mismatch is not an error — `OpenStore` handles it transparently by rebuilding content tables and clearing digests. Only genuine failures (unreadable file, I/O errors, corrupt metadata) cause `OpenStore` to return an error.

`Set` atomically creates or updates a catalog entry and returns the resulting `Catalog`. For new entries, `WithURI` is required — without a URI the catalog cannot be re-imported after a content rebuild. For existing entries, all options are optional; unspecified fields keep their current values. When `WithContent` is provided, the importer runs within the same transaction as the metadata update, and the digest is stored alongside the content. The entire `Set` call is atomic — metadata and content either both succeed or both roll back.

When content tables are rebuilt on open, all digests are cleared. The caller checks `Digest()` on each catalog to determine which need re-importing.

Metadata and content are independent — updating labels/priority does not require re-importing, and re-importing does not change other metadata fields.

### Importer and Writer interfaces

```go
type Importer interface {
    Import(ctx context.Context, w Writer) error
}

type PartialImportError interface {
    error
    PartialImport()
}

type Writer interface {
    InsertBundle(id, pkg, version, release, uri string) error
    CreateGraph(name string, parent *GraphID) (GraphID, error)
    AddBundleToGraph(graph GraphID, bundleID string) error
    AddSuccessor(graph GraphID, fromBundleID, toBundleID string) error
}

type GraphID int64
```

`Importer` is the contract for format-specific import logic. Library-olm ships importers for every catalog format OLM supports (currently FBC). The interface is exported because the normalized form (bundles, graphs, successors) is a well-defined contract — any source that can produce it is valid.

`Import` may return a `PartialImportError` to indicate that some items (e.g. packages) failed while others were successfully written. When `Set` receives a `PartialImportError`, it commits the transaction and propagates the error to the caller alongside the `Catalog` value. Any other error causes a rollback.

`Writer` is the normalized data contract. The `Store` implementation scopes all writes to the active catalog name and transaction. Importers never see SQL or the underlying schema.

### Schema details

**Metadata tables** (migratable):

Migration 1 (initial):
```sql
CREATE TABLE catalog_metadata (
    name   TEXT PRIMARY KEY,
    uri    TEXT NOT NULL DEFAULT '',
    digest TEXT NOT NULL DEFAULT ''
);

CREATE TABLE catalog_labels (
    catalog_name TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (catalog_name, key),
    FOREIGN KEY (catalog_name) REFERENCES catalog_metadata(name)
);

CREATE TABLE metadata_schema_version (
    version INTEGER NOT NULL
);
```

Migration 2 (add priority):
```sql
ALTER TABLE catalog_metadata ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;
```

**Content tables** (throw-away on mismatch):

```sql
CREATE TABLE content_schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE content_graphs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_name TEXT NOT NULL,
    name         TEXT NOT NULL,
    parent_id    INTEGER,
    FOREIGN KEY (parent_id) REFERENCES content_graphs(id),
    UNIQUE (catalog_name, name, parent_id)
);

CREATE TABLE content_bundles (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    catalog_name TEXT NOT NULL,
    bundle_id    TEXT NOT NULL,
    package_name TEXT NOT NULL DEFAULT '',
    version      TEXT NOT NULL DEFAULT '',
    release      TEXT NOT NULL DEFAULT '',
    uri          TEXT NOT NULL DEFAULT '',
    UNIQUE (catalog_name, bundle_id)
);

CREATE TABLE content_graph_bundles (
    graph_id  INTEGER NOT NULL,
    bundle_id INTEGER NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id),
    FOREIGN KEY (bundle_id) REFERENCES content_bundles(id)
);

CREATE TABLE content_successors (
    graph_id       INTEGER NOT NULL,
    from_bundle_id INTEGER NOT NULL,
    to_bundle_id   INTEGER NOT NULL,
    PRIMARY KEY (graph_id, from_bundle_id, to_bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id),
    FOREIGN KEY (from_bundle_id) REFERENCES content_bundles(id),
    FOREIGN KEY (to_bundle_id) REFERENCES content_bundles(id)
);
```

### Versioning

**Metadata schema** uses standard migrations. A `metadata_schema_version` table tracks the current version. `OpenStore` runs any pending migrations to bring the metadata schema up to date. Migrations are additive (add columns, add tables) and preserve existing data.

**Content schema** uses a simple version integer in `content_schema_version`. `OpenStore` checks it; on mismatch, all `content_*` tables are dropped and recreated with the new schema. The drop logic queries `sqlite_master` for tables matching the `content_%` prefix. No migration code for content — it's derived data that can be rebuilt from source.

A fingerprint test guarantees the content schema version is bumped when anything sensitive changes. The test imports a fixed fixture through the full pipeline, dumps the normalized content tables deterministically, hashes the result alongside the content schema DDL, and asserts it matches an expected constant. If the schema, handler logic, or normalization behavior changes, the hash changes and the test fails — forcing the developer to bump the content schema version and update the expected fingerprint.

### FBC importer

```go
func NewImporter(fsys fs.FS) catalogv1.Importer
```

The returned importer manages its own temporary SQLite database for staging. The `Import` method:

1. Creates a temporary SQLite DB file
2. Ingests FBC blobs into raw tables in the temp DB
3. Normalizes: reads from raw tables, writes through `Writer`
4. Deletes the temp DB

The FBC importer never touches the store's database directly — `Writer` is the only interface between the importer and the store. All importers (in-module and external) have exactly the same capabilities.

Package errors are returned from the importer as before (the error can be unwrapped into `fbc.PackageError` values).
