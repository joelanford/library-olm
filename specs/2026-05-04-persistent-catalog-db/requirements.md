# Requirements

## Metadata layer
- `catalogv1.Store` manages catalog metadata with name, URI, digest, priority, and labels
- `Set(ctx, name, opts...)` atomically creates or updates a catalog entry using functional options (`WithURI`, `WithPriority`, `WithLabels`, `WithContent`)
- `Set` on a new name requires `WithURI`; returns an error if URI is not provided
- `Set` on an existing name applies only the specified options; unspecified fields keep current values
- `WithContent(importer, digest)` pairs content import with digest update in the same transaction as metadata changes
- `Get` returns a `catalogv1.Catalog` with metadata and lazy content queries
- `Delete` removes a catalog entry and its associated content
- `List` returns all catalog entries as `[]Catalog`
- Metadata schema uses standard migrations — `OpenStore` always succeeds
- Metadata survives content schema changes

## Content layer
- `WithContent` executes the importer within the `Set` transaction, scoped to the catalog name
- `catalogv1.Writer` provides a type-safe interface for importers to write normalized data (bundles, graphs, successors)
- `catalogv1.Importer` is the interface for format-specific import logic
- `Catalog` content queries (`ListPackages`, `GetPackage`) return empty results if content has not been imported
- Content schema version is checked on `OpenStore`; on mismatch, all `content_*` tables are dropped and recreated
- When content tables are rebuilt on open, all digests are cleared; empty `Digest()` signals the catalog needs re-importing

## Catalog interface
- `catalogv1.Catalog` is extended with metadata methods: `Name`, `URI`, `Digest`, `Priority`, `Labels`
- `catalogv1.Catalog` retains content query methods: `ListPackages`, `GetPackage`
- `Metadata` struct is removed; metadata is read via `Catalog` methods

## Versioning
- Metadata schema version tracked and migrated forward automatically
- Content schema version tracked; mismatch triggers drop-and-recreate of all `content_*` tables
- Fingerprint test imports a fixed fixture, hashes normalized output with schema DDL, asserts against expected constant — guarantees content schema version is bumped when anything sensitive changes

## FBC importer
- `fbc.NewImporter(fsys)` returns a `catalogv1.Importer`
- FBC importer manages its own temporary SQLite DB for staging (never touches the store's DB directly)
- Returns per-package errors (unwrappable as `fbc.PackageError`) alongside partially successful imports

## Removals
- `fbc.FromFS` and `fbc.Catalog` are removed
- Normalized schema and query types move from `catalog/fbc/internal/` to `catalog/v1/internal/`
- `Store` implementation (`db` type) is unexported

## Acceptance Criteria

- `OpenStore` on a new path creates the DB with metadata and content schemas
- `OpenStore` on an existing DB migrates the metadata schema forward
- `OpenStore` on a content schema mismatch drops all `content_*` tables and recreates them, preserving metadata
- `Set` on a new name with `WithURI` creates the entry with defaults for unspecified fields
- `Set` on a new name without `WithURI` returns an error
- `Set` on an existing name updates only the specified fields, leaving others unchanged
- `Set` with `WithContent` imports content and stores digest atomically with metadata changes
- `Set` is atomic — metadata and content either both succeed or both roll back
- `Get` returns a `Catalog` with correct metadata; content queries work if content exists
- `Get` returns an error for nonexistent names
- `Delete` removes the entry and all associated content
- `List` returns all entries as `[]Catalog` with correct metadata
- Multiple catalogs in the same store are fully isolated (no cross-contamination in queries)
- After content schema rebuild, all digests are empty; after re-importing, digests are populated
- `fbc.NewImporter` produces identical query results to the old `fbc.FromFS` for the same input
- `fbc.NewImporter` returns per-package errors (via `PartialImportError`) for malformed packages while importing valid ones; `Set` commits and propagates the error
- Fingerprint test fails when content schema DDL or normalization logic changes without a version bump
- All existing tests pass after the refactoring (with necessary updates to use the new API)
