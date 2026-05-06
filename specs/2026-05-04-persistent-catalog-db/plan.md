# Implementation Plan

1. **Create `catalog/v1/internal/` with content schema and query types**
   - Move `BundleRow` and query types (`CatalogQuery`, `UpdateGraphQuery`, `CompositeUpdateGraphQuery`, and supporting functions) from `catalog/fbc/internal/` to `catalog/v1/internal/`
   - Move the normalized schema DDL (`graphs`, `bundles`, `graph_bundles`, `successors` tables) to `catalog/v1/internal/`
   - Add `catalog_name TEXT NOT NULL` column to all content tables
   - Add `content_schema_version` table
   - Update primary keys, unique constraints, and indexes to include `catalog_name`
   - Update all queries to filter by `catalog_name`
   - Define content `SchemaVersion` constant (initial value: `1`)

2. **Add metadata schema and migration support in `catalog/v1/internal/`**
   - Implement migration runner: read current version, apply pending migrations sequentially, update version
   - Migration 1 (initial): create `catalog_metadata` table (name, uri, digest), `catalog_labels` table (catalog_name, key, value), and `metadata_schema_version` table
   - Migration 2: add `priority INTEGER NOT NULL DEFAULT 0` column to `catalog_metadata`

3. **Define exported types and interfaces in `catalog/v1/`**
   - Extend `Catalog` interface with metadata methods (`Name`, `URI`, `Digest`, `Priority`, `Labels`) alongside existing content methods (`ListPackages`, `GetPackage`)
   - Define `Store` interface with `Set`, `Get`, `Delete`, `List`, `Close`
   - Define `SetOption` type and option constructors (`WithURI`, `WithPriority`, `WithLabels`, `WithContent`)
   - Define `Importer` interface with `Import(ctx, Writer) error`
   - Define `Writer` interface with `InsertBundle`, `CreateGraph`, `AddBundleToGraph`, `AddSuccessor`
   - Define `GraphID` type
   - Define `OpenStore(path) (Store, error)` constructor
   - Implement the concrete `Writer` in `catalog/v1/internal/` that writes to content tables scoped by catalog name within a transaction
   - Implement `Catalog` in `catalog/v1/internal/` backed by metadata fields and lazy content queries

4. **Implement unexported `db` type in `catalog/v1/`**
   - `OpenStore(path)` — open/create SQLite file, set pragmas, run metadata migrations, check content schema version (drop all `content_*` tables and recreate on mismatch, clear digests)
   - `Set(ctx, name, opts...)` — check if entry exists; if new, require `WithURI` and insert with defaults; if existing, apply only specified options; if `WithContent` is provided, delete existing content, create `Writer`, call importer, store digest; all within a single transaction
   - `Get(name)` — query metadata and return a `Catalog` with lazy content queries
   - `Delete(name)` — delete metadata, labels, and all associated content
   - `List()` — query all entries joined with labels, return `[]Catalog` with lazy content queries
   - Content schema rebuild clears all digests in `catalog_metadata`
   - `Close()` — close the `*sql.DB`

5. **Refactor FBC importer to use `Writer`**
   - Create `fbc.NewImporter(fsys fs.FS) catalogv1.Importer`
   - The importer's `Import` method: create a temporary SQLite DB, ingest FBC blobs into raw tables in the temp DB, normalize by reading raw tables and writing through `Writer`, delete temp DB
   - Update `OLMPackageHandler.Normalize` to write through `Writer` instead of direct SQL on the store's DB
   - Return per-package errors from the importer
   - Keep raw table DDL, ingest logic, and handler dispatch in `catalog/fbc/internal/`

6. **Remove old `fbc.FromFS`, `fbc.Catalog`, and `fbc.Close`**
   - Delete `fbc.Catalog` struct and its methods
   - Delete `fbc.FromFS` function
   - Remove `internal.OpenDB`, `internal.CloseDB` (DB lifecycle now owned by `catalogv1` store)
   - Remove the old normalized schema DDL from `catalog/fbc/internal/`
   - Clean up compile-time interface assertions that referenced the old types

7. **Update tests and add fingerprint test**
   - Update `catalog/fbc/catalog_test.go` to use `catalogv1.OpenStore` + `store.Set` with `WithURI`/`WithContent` + `store.Get`
   - Update `catalog/fbc/example_test.go` similarly
   - Add tests for metadata operations: set (new with URI, new without URI error, update partial fields, set with content + digest), get, list, delete, labels
   - Add tests for content operations: catalog queries, multi-catalog isolation, content replacement, empty digest after content rebuild, empty content queries for catalogs without content
   - Add tests for schema lifecycle: metadata migration, content schema mismatch rebuild, metadata preserved across rebuild
   - Add fingerprint test: import fixed fixture, dump normalized content tables, hash with content schema DDL, assert against expected constant
   - Verify per-package error behavior is preserved
