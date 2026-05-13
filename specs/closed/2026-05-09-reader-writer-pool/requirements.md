# Requirements

- Content DB (`catalog/v1/`) uses separate reader and writer `*sql.DB` pools sharing the same file
- FBC importer's staging DB (`catalog/fbc/internal/`) uses separate reader and writer `*sql.DB` pools sharing the same temp file
- Writer pools use `MaxOpenConns(1)` for write serialization
- Reader pools allow unlimited concurrent connections
- Reader pools use `mode=ro` in the DSN to enforce read-only access at the SQLite level
- Both pools use WAL mode, `busy_timeout=5000`, `synchronous=NORMAL`, and `foreign_keys=ON`
- Both pools set pragmas via DSN `_pragma` parameters (not `Exec`)
- Content DB iterators (`ListPackages`, `ListGraphs`, `ListBundles`, `Successors`) stream rows directly instead of pre-collecting
- Content DB `List()` streams catalog metadata rows directly instead of pre-collecting
- FBC staging DB accessor iterators (`Bundles`, `Channels`, `Deprecations`, `Others`, `Entries`) stream rows directly instead of pre-collecting
- `BundleRow.Property()` and graph `Property()` work correctly when called inside streaming iterators
- `Close` / `CloseTempDB` close both pools
- `ListPackages`, `GetPackage`, `ListGraphs`, `GetGraph` use a single parameterized SQL helper — no assumption that packages are always composite or children are always leaf
- `Set` returns metadata read from the write transaction, not from a post-commit reader pool query
- Shared `querier` interface, `getCatalog`, and `queryLabels` helpers eliminate duplication between `Set` and `Get`

## Acceptance Criteria

- All existing tests pass without modification (behavioral equivalence)
- No deadlocks when nesting read iterators (e.g., iterating bundles while calling `Property()`)
- No deadlocks when nesting FBC staging DB accessors (e.g., iterating channels inside a bundles loop)
- No TOCTOU race in `Set` return value under concurrent writers
- Pre-collection helper functions (`collectBundles`, `collectChannels`, `collectBundleResults`, etc.) and their associated intermediate types (`bundleResult`, `compositeUpdateGraphResult`, `updateGraphResult`) are removed
- `make ci` passes (lint, test, build)
