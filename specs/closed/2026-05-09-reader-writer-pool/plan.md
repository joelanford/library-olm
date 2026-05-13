# Implementation Plan

## 1. Content DB reader/writer split

Split the content DB from one `*sql.DB` to two, keeping all existing pre-collection logic intact
to verify no regressions before changing iteration behavior.

- Rename `db.sqlDB` to `db.writerDB` in `catalog/v1/db.go`
- Add `db.readerDB *sql.DB` field
- In `OpenStore`:
  - Open the writer pool using DSN format: `file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)`, `MaxOpenConns(1)`
  - Open the reader pool using DSN format: `file:<path>?mode=ro&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)`
  - No `MaxOpenConns` limit on reader pool
  - Run migrations and schema rebuilds on the writer pool (they are write operations)
  - Schema version check (`CheckContentSchemaVersion`) is a read but runs at startup before
    the reader pool is needed — using the writer pool is fine and avoids opening the reader
    before the schema is known-good
- Route writes through `writerDB`: `Set()`, `Delete()` transactions
- Route reads through `readerDB`: `Get()`, `List()`, `queryLabels()`, query construction
  (`CatalogQuery`, `CompositeUpdateGraphQuery`, `UpdateGraphQuery`, `BundleRow` all receive
  `readerDB` via their `DB` field)
- `Close()`: close both pools (close reader first, then writer)
- Run `make ci` — all tests must pass with pre-collection still in place

## 2. Content DB streaming conversion

Replace pre-collection with direct streaming in the content DB query layer.

- **`catalog/v1/internal/query.go`:**
  - Add `GraphNode` type with `DB`, `CatalogName`, `ID`, `Name`, `Path`, `HasChildren` fields
  - Add `queryGraphNodes` helper that builds a single parameterized SQL query from optional
    `parentID` (nil = top-level, non-nil = children) and `name` (empty = list, non-empty = get)
    parameters, with `parentPath` for hierarchical error context
  - `CatalogQuery.ListPackages` / `GetPackage`: delegate to `queryGraphNodes` with `parentID=nil`
  - `CompositeUpdateGraphQuery.ListGraphs` / `GetGraph`: delegate to `queryGraphNodes` with
    `parentID=&graphID`
  - `queryBundlesDirect` / `queryBundlesDescendant`: stream rows directly, parse and yield each
    `BundleRow` immediately (remove `collectBundleResults`)
  - `querySuccessorsCollected`: stream explicit successors first (yield immediately, track seen
    IDs), then stream range successors (skip seen, yield immediately). Remove
    `collectSuccessorResults`, `collectExplicitSuccessorResults`, `collectRangeSuccessorResults`
  - Remove intermediate types: `bundleResult`, `compositeUpdateGraphResult`,
    `updateGraphResult`, `yieldBundleResults`
- **`catalog/v1/db.go`:**
  - Add `wrapGraphNode` / `wrapGraphNodes` to convert `GraphNode` to `UpdateGraph` based on
    `HasChildren`, wrapping as `compositeUpdateGraphWrapper` or `*UpdateGraphQuery`
  - Simplify `ListPackages`, `GetPackage`, `ListGraphs`, `GetGraph` wrappers to use
    `wrapGraphNode` / `wrapGraphNodes`
  - `db.List()`: stream catalog metadata rows directly, call `queryLabels` per row inside the
    loop (safe with multiple reader connections)
- Run `make ci`

## 2a. TOCTOU fix and query helpers

Fix `Set` to read catalog metadata within the write transaction before committing.

- **`catalog/v1/db.go`:**
  - Add `querier` interface (satisfied by `*sql.DB` and `*sql.Tx`) with `Query` and `QueryRow`
  - Extract `queryLabels(q querier, name)` standalone function (replaces `d.queryLabels` method)
  - Extract `getCatalog(q querier, readerDB, name)` helper shared by `Set` (via `tx`) and `Get`
    (via `readerDB`)
  - `Set`: call `getCatalog(tx, d.readerDB, name)` before `tx.Commit()` to return data
    consistent with what was written
  - `Get`: one-liner delegating to `getCatalog(d.readerDB, d.readerDB, name)`
- Run `make ci`

## 3. FBC staging DB reader/writer split

Split the FBC importer's staging DB from one `*sql.DB` to two, keeping pre-collection intact.

- Modify `OpenTempDB` in `catalog/fbc/internal/db.go` to return both writer and reader pools:
  - Writer pool: DSN format `file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)`, `MaxOpenConns(1)`, schema creation
  - Reader pool: DSN format `file:<path>?mode=ro&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)`, unlimited connections
  - Return signature: `(writerDB, readerDB *sql.DB, tmpDir string, err error)`
- Modify `CloseTempDB` to accept and close both pools
- Update `catalog/fbc/importer.go`:
  - `Import`: pass writer pool to `Ingest`, reader pool to `Normalize` and `finalize`
  - `finalize`: use reader pool for `NewPackageAccessor`
- Update `catalog/fbc/internal/normalize.go`:
  - `Normalize`: accept reader pool for package listing and `PackageAccessor` construction,
    writer pool is not used here (writes go through the `catalogv1.Writer` which operates on the
    content DB)
- Run `make ci`

## 4. FBC staging DB streaming conversion

Replace pre-collection with direct streaming in the FBC staging DB accessor layer.

- **`catalog/fbc/internal/accessor.go`:**
  - `PackageAccessor.Bundles()`: stream query rows directly, yield each `bundleAccessor`
    immediately (remove `collectBundles`)
  - `PackageAccessor.Channels()`: stream query rows directly, yield each `channelAccessor`
    immediately (remove `collectChannels`)
  - `PackageAccessor.Deprecations()`: stream query rows directly (remove `collectDeprecations`)
  - `PackageAccessor.Others()`: stream query rows directly (remove `collectOthers`)
  - `channelAccessor.Entries()`: stream query rows directly (remove `collectEntries`)
  - Remove all `collect*` helper methods
- Run `make ci`
