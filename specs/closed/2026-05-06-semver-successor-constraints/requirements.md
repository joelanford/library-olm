# Requirements

- `Successors` on `UpdateGraph` and `CompositeUpdateGraph` changes signature from `Successors(ctx, from BundleID)` to `Successors(ctx, fromID BundleID, fromVersion bsemver.Version)`. `fromID` is used for explicit edge lookup; `fromVersion` is used for predecessor range evaluation.
- The `Writer` interface has an `AddPredecessorRange(graph GraphID, bundleID, versionRange string) error` method that stores a blang/semver range for a bundle in a graph.
- `AddPredecessorRange` validates the range string with `bsemver.ParseRange()` and returns an error if unparseable.
- At most one range per bundle per graph (primary key `(graph_id, bundle_id)`). Duplicates are prevented by FBC-level channel entry uniqueness validation.
- `Successors()` returns the union of explicit edges (from `content_successors`, looked up by `fromID`) and range-based matches (from `content_predecessor_ranges`, evaluated against `fromVersion`), deduplicated by bundle ID.
- Range evaluation: parse with `bsemver.ParseRange()`, check with `rng(fromVersion)`. The caller is responsible for providing the `bsemver.Version`.
- Range evaluation always runs; callers are expected to provide a valid version.
- If a stored range is unparseable at query time, `Successors()` yields an error for that row (does not silently skip).
- Both leaf-graph (`querySuccessorsDirect`) and composite-graph (`querySuccessorsDescendant`) query paths support range evaluation.
- `ContentSchemaVersion` is bumped to 3.
- `DeleteCatalogContent` includes the new table in its FK-safe delete order.

## Acceptance Criteria

- A bundle with a range `>=1.0.0 <2.0.0` in a graph is returned as a successor when called with version `1.5.0`.
- The same bundle is NOT returned when called with version `2.0.0` or `0.9.0`.
- A bundle reachable via both an explicit edge (by ID) and a range match (by version) appears only once in the `Successors()` output.
- Calling with version `0.0.0` evaluates ranges against `0.0.0` (no special-casing of zero value).
- Calling with a BundleID not in the catalog returns no explicit edges, but range matches based on the provided version still work.
- `AddPredecessorRange` with an invalid range string (e.g. `"not a range"`) returns an error.
- A range using blang `||` syntax (e.g. `>=1.0.0 <2.0.0 || >=3.0.0`) correctly matches versions in either range.
- Existing explicit-only successor behavior is unaffected (regression tests pass with updated call sites).
- Content schema version mismatch triggers a drop-and-rebuild cycle that creates the new table.
