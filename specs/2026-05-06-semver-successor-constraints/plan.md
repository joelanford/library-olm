# Implementation Plan

## 1. Successors signature change

- In `catalog/v1/catalog.go`:
  - Change `Successors(ctx context.Context, from bundlev1.BundleID)` to `Successors(ctx context.Context, id bundlev1.BundleID, version mmsemver.Version)` on both `UpdateGraph` and `CompositeUpdateGraph`.
- In `catalog/v1/internal/query.go`:
  - Update `UpdateGraphQuery.Successors` and `CompositeUpdateGraphQuery.Successors` to accept the new parameters.
  - Update `querySuccessorsDirect` and `querySuccessorsDescendant` to accept `id BundleID` and `version mmsemver.Version` (pass `id` through for explicit edge lookup, `version` used later for constraints).
- Update all call sites (tests in `catalog/v1/db_test.go`, any FBC tests) to pass both parameters.
- Verify `make ci` passes with the signature change alone before proceeding.

## 2. Schema and writer

- In `catalog/v1/internal/content.go`:
  - Add `content_predecessor_constraints` table and index to `contentSchemaSQL`.
  - Bump `ContentSchemaVersion` from 1 to 2.
- In `catalog/v1/store.go`:
  - Add `AddPredecessorConstraint(graph GraphID, bundleID, constraint string) error` to the `Writer` interface.
- In `catalog/v1/internal/writer.go`:
  - Implement `AddPredecessorConstraint` on `ContentWriter`: validate with `semver.NewConstraint()`, then `INSERT OR REPLACE INTO content_predecessor_constraints`.
  - Update `DeleteCatalogContent` to delete from `content_predecessor_constraints` before `content_successors` in the FK-safe order.
- Run `go get github.com/Masterminds/semver/v3` to promote the dependency from indirect to direct.

## 3. Query-time evaluation

- In `catalog/v1/internal/query.go`:
  - Update `querySuccessorsDirect` to:
    - Yield explicit edges first (existing logic, using `id`).
    - Track yielded bundle IDs in a `map[string]bool`.
    - Query all `(bundle_id_int, version_constraint)` from `content_predecessor_constraints` for this graph.
    - For each row, parse constraint with `semver.NewConstraint()`, check against `version`.
    - Yield matching bundles not already seen.
  - Update `querySuccessorsDescendant` similarly, using the recursive CTE to collect descendant graph IDs for both explicit edges and constraint lookups.

## 4. Tests

Tests go in `catalog/v1/db_test.go` (external test package `catalogv1_test`), following the existing convention of testing through the public `Store` API using `testImporter` functions that exercise `Writer` methods.

- Test `AddPredecessorConstraint` with a valid Masterminds constraint string.
- Test `AddPredecessorConstraint` with an invalid constraint string returns an error.
- Test `AddPredecessorConstraint` with the same `(graph, bundle)` replaces the previous constraint.
- Test `Successors()` with constraint-only edges: bundles within range returned, bundles outside range not returned.
- Test `Successors()` with explicit-only edges: existing behavior unchanged with new signature.
- Test `Successors()` with both explicit and constraint edges: union is returned, no duplicates.
- Test `Successors()` with version `0.0.0`: constraints evaluated against `0.0.0` normally (no special-casing).
- Test `Successors()` with BundleID not in catalog: no explicit edges, constraint matches by version still returned.
- Test Masterminds `||` syntax: `>= 1.0.0, < 2.0.0 || >= 3.0.0` matches `1.5.0` and `3.1.0` but not `2.5.0`.
- Test composite graph (descendant) variant with constraints.
- Run `make ci` to verify lint, tests, and build all pass.
