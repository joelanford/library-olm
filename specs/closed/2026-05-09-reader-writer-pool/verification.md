# Verification

## Implementation Correctness

- [ ] Content DB `OpenStore` opens two `*sql.DB` handles to the same file
- [ ] Content DB writer pool has `MaxOpenConns(1)`
- [ ] Content DB reader pool has no `MaxOpenConns` limit
- [ ] Reader pools use `mode=ro` in the DSN to enforce read-only at the SQLite level
- [ ] Both reader and writer pools use DSN `_pragma` parameters for pragma initialization (no `Exec`-based pragma setting)
- [ ] All content DB write operations (`Set`, `Delete`, migrations, schema rebuilds) use the writer pool
- [ ] Startup schema check runs on the writer pool (it's a read, but precedes reader pool creation and gates potential rebuilds)
- [ ] All content DB read operations (`Get`, `List`, `queryLabels`, query types) use the reader pool
- [ ] `BundleRow.DB`, `CatalogQuery.DB`, `CompositeUpdateGraphQuery.DB`, `UpdateGraphQuery.DB` all receive the reader pool
- [ ] `Close` closes both pools
- [ ] FBC staging DB `OpenTempDB` returns both writer and reader pools
- [ ] FBC staging DB writer pool used for ingest, reader pool used for normalize and finalize
- [ ] `CloseTempDB` closes both pools
- [ ] All `collect*` pre-collection functions are removed from both `query.go` and `accessor.go`
- [ ] All intermediate result types (`bundleResult`, `compositeUpdateGraphResult`, `updateGraphResult`) are removed
- [ ] Successor query streaming correctly deduplicates explicit and range-based results via a `seen` map
- [ ] `db.List()` streams metadata rows and queries labels per-row instead of pre-collecting
- [ ] `ListPackages`, `GetPackage`, `ListGraphs`, `GetGraph` all use a single `queryGraphNodes` helper
- [ ] `queryGraphNodes` builds SQL from optional `parentID` and `name` parameters (no duplicated SQL)
- [ ] `GraphNode.HasChildren` determined via `EXISTS` subquery per row
- [ ] `GraphNode.Path` carries full hierarchy from root for contextual error messages
- [ ] `wrapGraphNode` returns `compositeUpdateGraphWrapper` or `*UpdateGraphQuery` based on `HasChildren`
- [ ] `Set` reads metadata and labels from `tx` before committing (no post-commit read via `readerDB`)
- [ ] `querier` interface satisfied by both `*sql.DB` and `*sql.Tx`
- [ ] `getCatalog` and `queryLabels` shared by `Set` (via `tx`) and `Get` (via `readerDB`)

## Project Conventions

- [ ] `make ci` passes (lint, test, build)
- [ ] No new public API surface (all changes are internal to existing packages)
- [ ] DSN construction uses `fmt.Sprintf` or equivalent — no manual string concatenation with user-provided paths (handle paths with special characters)
- [ ] No `//nolint` comments added
- [ ] Pure data types with standalone functions — no methods coupling logic to data (per `specs/mission.md`)
- [ ] Internal packages used for implementation details (per `specs/mission.md`)
- [ ] One logical change per commit (per `specs/conventions.md`)
