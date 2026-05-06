# Implementation Plan

## 1. Successors signature change

- In `catalog/v1/catalog.go`:
  - Change `Successors(ctx context.Context, from bundlev1.BundleID)` to `Successors(ctx context.Context, fromID bundlev1.BundleID, fromVersion bsemver.Version)` on both `UpdateGraph` and `CompositeUpdateGraph`.
- In `catalog/v1/internal/query.go`:
  - Update `UpdateGraphQuery.Successors` and `CompositeUpdateGraphQuery.Successors` to accept the new parameters.
  - Update `querySuccessorsDirect` and `querySuccessorsDescendant` to accept `fromID BundleID` and `fromVersion bsemver.Version` (pass `fromID` through for explicit edge lookup, `fromVersion` used later for ranges).
- Update all call sites (tests in `catalog/v1/db_test.go`, any FBC tests) to pass both parameters.
- Verify `make ci` passes with the signature change alone before proceeding.

## 2. Schema and writer

- In `catalog/v1/internal/content.go`:
  - Add `content_predecessor_ranges` table and index to `contentSchemaSQL`.
  - Bump `ContentSchemaVersion` from 1 to 3.
- In `catalog/v1/store.go`:
  - Add `AddPredecessorRange(graph GraphID, bundleID, versionRange string) error` to the `Writer` interface.
- In `catalog/v1/internal/writer.go`:
  - Implement `AddPredecessorRange` on `ContentWriter`: validate with `bsemver.ParseRange()`, then `INSERT INTO content_predecessor_ranges`.
  - Update `DeleteCatalogContent` to delete from `content_predecessor_ranges` before `content_successors` in the FK-safe order.

## 3. Query-time evaluation

- In `catalog/v1/internal/query.go`:
  - Update `querySuccessorsDirect` to:
    - Yield explicit edges first (existing logic, using `fromID`).
    - Track yielded bundle IDs in a `map[string]bool`.
    - Query all `(bundle_id, version_range)` from `content_predecessor_ranges` for this graph.
    - For each row, parse range with `bsemver.ParseRange()`, check against `fromVersion`.
    - Yield matching bundles not already seen.
  - Update `querySuccessorsDescendant` similarly, using the recursive CTE to collect descendant graph IDs for both explicit edges and range lookups.

## 4. Tests

Tests go in `catalog/v1/db_test.go` (external test package `catalogv1_test`), following the existing convention of testing through the public `Store` API using `testImporter` functions that exercise `Writer` methods.

- Test `AddPredecessorRange` with a valid blang range string.
- Test `AddPredecessorRange` with an invalid range string returns an error.
- Test `Successors()` with range-only edges: bundles within range returned, bundles outside range not returned.
- Test `Successors()` with explicit-only edges: existing behavior unchanged with new signature.
- Test `Successors()` with both explicit and range edges: union is returned, no duplicates.
- Test `Successors()` with version `0.0.0`: ranges evaluated against `0.0.0` normally (no special-casing).
- Test `Successors()` with BundleID not in catalog: no explicit edges, range matches by version still returned.
- Test blang `||` syntax: `>=1.0.0 <2.0.0 || >=3.0.0` matches `1.5.0` and `3.1.0` but not `2.5.0`.
- Test composite graph (descendant) variant with ranges.
- Run `make ci` to verify lint, tests, and build all pass.
