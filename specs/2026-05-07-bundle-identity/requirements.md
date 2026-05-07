# Requirements

- `BundleIdentity` is a new interface in `bundle/v1` with `ID() BundleID` and `NameVersionRelease() NameVersionRelease`.
- `Bundle` embeds `BundleIdentity` and adds `URI() string`.
- `UpdateGraph.Successors` accepts a single `BundleIdentity` parameter instead of separate `BundleID` and `bsemver.Version` parameters.
- `CompositeUpdateGraph.Successors` accepts the same `BundleIdentity` parameter.
- The internal query layer unpacks `from.ID()` for explicit-edge queries and `from.NameVersionRelease().Version` for predecessor-range queries.
- `CompositeUpdateGraph.GetGraph` returns a `CompositeUpdateGraph` when the child graph has sub-graphs, enabling arbitrary-depth path walking. This is determined by a single SQL query (`SELECT ... EXISTS(SELECT 1 FROM content_graphs WHERE parent_id = ...)`).
- `internal/util/test` provides `BundleIdentity` (exported struct with `BundleID` and `NVR` fields) and `NewBundleIdentity(t, name, version, release)` for test code.
- `NewBundleIdentity` derives the bundle ID as `{name}.v{version}[-{release}]`. Tests needing a custom ID use the struct directly.
- `importas` lint rule enforces `internal/util/<name>` packages are aliased as `<name>util` via regex pattern.

## Acceptance Criteria

- Callers with a `Bundle` can pass it directly to `Successors` without extracting fields.
- All existing `Successors` call sites compile and pass tests with the new signature.
- `GetGraph` returns `CompositeUpdateGraph` for graphs with children, enabling depth >1 path walking in resolver.
- No duplicate `testBundleIdentity` types across test packages — all use `internal/util/test`.
- `make ci` passes.
