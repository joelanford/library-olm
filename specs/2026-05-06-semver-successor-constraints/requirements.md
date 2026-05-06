# Requirements

- `Successors` on `UpdateGraph` and `CompositeUpdateGraph` changes signature from `Successors(ctx, from BundleID)` to `Successors(ctx, id BundleID, version mmsemver.Version)`. The `id` is used for explicit edge lookup; `version` is used for predecessor constraint evaluation.
- The `Writer` interface has an `AddPredecessorConstraint(graph GraphID, bundleID, constraint string) error` method that stores a Masterminds semver constraint for a bundle in a graph.
- `AddPredecessorConstraint` validates the constraint string with `semver.NewConstraint()` and returns an error if unparseable.
- At most one constraint per bundle per graph (primary key `(graph_id, bundle_id)`). A second call for the same `(graph, bundle)` is an `INSERT OR REPLACE`.
- `Successors()` returns the union of explicit edges (from `content_successors`, looked up by `id`) and constraint-based matches (from `content_predecessor_constraints`, evaluated against `version`), deduplicated by bundle ID.
- Constraint evaluation: check each constraint with `constraint.Check(version)`. The caller is responsible for constructing the `mmsemver.Version`.
- Constraint evaluation always runs; callers are expected to provide a valid version.
- If a stored constraint is unparseable at query time, `Successors()` yields an error for that row (does not silently skip).
- Both leaf-graph (`querySuccessorsDirect`) and composite-graph (`querySuccessorsDescendant`) query paths support constraint evaluation.
- `ContentSchemaVersion` is bumped to 2.
- `DeleteCatalogContent` includes the new table in its FK-safe delete order.
- `Masterminds/semver/v3` becomes a direct dependency.

## Acceptance Criteria

- A bundle with a constraint `>= 1.0.0, < 2.0.0` in a graph is returned as a successor when called with version `1.5.0`.
- The same bundle is NOT returned when called with version `2.0.0` or `0.9.0`.
- A bundle reachable via both an explicit edge (by ID) and a constraint match (by version) appears only once in the `Successors()` output.
- Calling with version `0.0.0` evaluates constraints against `0.0.0` (no special-casing of zero value).
- Calling with a BundleID not in the catalog returns no explicit edges, but constraint matches based on the provided version still work.
- `AddPredecessorConstraint` with an invalid constraint string (e.g. `"not a constraint"`) returns an error.
- A constraint using Masterminds `||` syntax (e.g. `>= 1.0.0 < 2.0.0 || >= 3.0.0`) correctly matches versions in either range.
- Existing explicit-only successor behavior is unaffected (regression tests pass with updated call sites).
- Content schema version mismatch triggers a drop-and-rebuild cycle that creates the new table.
