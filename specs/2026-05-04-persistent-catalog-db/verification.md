# Verification

## Implementation Correctness

### Metadata layer
- [ ] `OpenStore` creates a valid DB with metadata and content schemas
- [ ] `OpenStore` migrates the metadata schema forward on existing DBs
- [ ] Metadata is preserved when content tables are rebuilt
- [ ] `Set` on a new name with `WithURI` creates entry with defaults for unspecified fields
- [ ] `Set` on a new name without `WithURI` returns an error
- [ ] `Set` on an existing name updates only specified fields, leaving others unchanged
- [ ] `Set` is atomic (no read-modify-write race)
- [ ] `Set` with `WithContent` imports content and stores digest in the same transaction
- [ ] `Set` with `WithContent` rolls back both metadata and content on importer error
- [ ] `Get` returns a `Catalog` with correct metadata and working content queries
- [ ] `Get` returns an error for nonexistent name
- [ ] `Delete` removes the entry, labels, and all associated content
- [ ] `List` returns all entries as `[]Catalog` with correct metadata and lazy content queries

### Content layer
- [ ] Content queries return empty results for catalogs without imported content
- [ ] Content queries return correct results for catalogs with imported content
- [ ] Multiple catalogs in one store are fully isolated in queries
- [ ] Re-importing content (via `Set` with `WithContent`) replaces previous content
- [ ] Content schema rebuild clears all digests
- [ ] `Digest()` returns empty for catalogs that have not been imported
- [ ] `Digest()` returns the correct value after importing with `WithContent`
- [ ] `Close` releases the database connection without deleting the file

### Catalog interface
- [ ] `Catalog` exposes metadata via `Name`, `URI`, `Digest`, `Priority`, `Labels` methods
- [ ] `Catalog` exposes content via `ListPackages`, `GetPackage` methods
- [ ] Content queries are lazy (execute on demand, not on `Get`/`List`)

### Versioning
- [ ] Content schema mismatch drops all `content_*` tables and recreates them
- [ ] Metadata schema migrations run on open and bring schema up to date
- [ ] Migration 2 adds `priority` column to existing DBs without data loss
- [ ] Fingerprint test fails when content schema DDL or normalization logic changes without a version bump

### FBC importer
- [ ] `fbc.NewImporter` produces the same query results as the old `fbc.FromFS` for identical input
- [ ] `fbc.NewImporter` returns per-package errors as a `PartialImportError` from `Import`; `Set` commits and propagates the error alongside the `Catalog` value
- [ ] FBC importer's temporary staging DB is cleaned up after import (success or failure)
- [ ] All existing tests pass with updated API calls

## Project Conventions

- [ ] Commits follow conventional commit format: `refactor:`, `feat:`, etc.
- [ ] One logical change per commit matching the implementation plan task groups
- [ ] Public API additions have tests with at least 70% statement coverage
- [ ] No cluster dependencies introduced (no kubeconfig, kube client, controller-runtime)
- [ ] Legacy dependency usage not increased (`operator-framework/api`, `operator-framework/operator-registry`)
- [ ] Pure data types with standalone functions — store methods are query/lifecycle, not data transformation
- [ ] Internal packages used for implementation details (`catalog/v1/internal/`)
- [ ] `Store` implementation (`db` type) is unexported
- [ ] `make ci` passes (lint, test, build)
