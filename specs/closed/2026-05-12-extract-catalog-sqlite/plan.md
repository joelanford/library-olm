# Implementation Plan

## 1. Create catalog/v1/sqlite package with moved files

1. Create `catalog/v1/sqlite/` directory
2. Copy internal files into `sqlite/`:
   - `internal/content.go` → `sqlite/content.go`
   - `internal/metadata.go` → `sqlite/metadata.go`
   - `internal/query.go` → `sqlite/query.go`
   - `internal/writer.go` → `sqlite/writer.go`
3. Change `package internal` to `package sqlite`
4. Unexport all exported symbols
5. Add `catalog/v1` import for interface types where needed

## 2. Introduce shared graph query base (sqlite/query.go)

Replace `UpdateGraphQuery` and `CompositeUpdateGraphQuery` with:

1. **`graphQuery`** — holds `{db, catalogName, graphID, graphName}`.
   Methods: `Name()`, `Property()`, `ListBundles()` (direct),
   `Successors()` (direct). Implements `catalogv1.UpdateGraph`.

2. **`compositeGraphQuery`** — embeds `graphQuery`, adds `graphPath`.
   Shadows `ListBundles()` (descendant), `Successors()` (descendant).
   Adds `ListGraphs()`, `GetGraph()`. Implements
   `catalogv1.CompositeUpdateGraph`.

## 3. Eliminate graphNode, convert queryGraphNodes (sqlite/query.go)

1. Delete the `GraphNode` type.
2. Change `queryGraphNodes` to return
   `iter.Seq2[catalogv1.UpdateGraph, error]`. Inside the iterator,
   construct `*graphQuery` or `*compositeGraphQuery` directly based on
   the `HasChildren` scan.
3. Add `queryGraphNode` (singular) returning
   `(catalogv1.UpdateGraph, error)` for single-result callers.
4. Delete `CatalogQuery` — its two methods are inlined into
   `storedCatalog`.

## 4. Fold DeleteCatalogContent into contentWriter (sqlite/writer.go)

Make it a `deleteAll()` method on `contentWriter`.

## 5. Move implementation from db.go to sqlite/sqlite.go

1. Move `OpenStore`, `db`, `sqliteDSN`, `storedCatalog`,
   `selectedStore`, `andSelector`, `querier`, `getCatalog`,
   `queryLabels` into `sqlite/sqlite.go`.
2. Move `labelCatalogName` from `store.go` to `sqlite/sqlite.go`.
3. Add `readerDB *sql.DB` field to `storedCatalog`. `ListPackages`
   calls `queryGraphNodes` directly. `GetPackage` calls
   `queryGraphNode` directly.
4. `contentWriter` implements `catalogv1.Writer` directly — no
   `writerAdapter`.
5. `compositeGraphQuery` implements `catalogv1.CompositeUpdateGraph`
   directly — no `compositeUpdateGraphWrapper`.
6. Do NOT copy: `writerAdapter`, `compositeUpdateGraphWrapper`,
   `wrapGraphNode`, `wrapGraphNodes`. These are eliminated.
7. Update compile-time interface checks.

## 6. Delete old files

1. Delete `catalog/v1/internal/` directory.
2. Delete `catalog/v1/db.go` (all contents have moved).
3. Keep only `catalog.go` and `store.go` in `catalog/v1/`.

## 7. Move tests

Move `db_test.go`, `db_deprecation_test.go`, `db_fingerprint_test.go`
to `catalog/v1/sqlite/`. Update imports from `catalogv1.OpenStore` to
`sqlite.OpenStore` (or use `_test` package importing both).

## 8. Update external callers

Update all callers of `catalogv1.OpenStore` to import
`catalog/v1/sqlite` and call `sqlite.OpenStore`:
- `examples/query_operatorhubio/main.go`
- `catalog/fbc/` test files
- `resolver/v1/` test files

## 9. Update specs/tech-stack.md

Update the project structure to reflect the new layout.

## 10. Verify

Run `make ci` to confirm lint, test, and build all pass.
