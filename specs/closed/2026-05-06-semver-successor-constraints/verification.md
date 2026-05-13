# Verification

## Implementation Correctness

- [x] `Successors` signature on `UpdateGraph` and `CompositeUpdateGraph` is `Successors(ctx, fromID BundleID, fromVersion bsemver.Version)`.
- [x] All call sites updated for the new signature.
- [x] `content_predecessor_ranges` table exists in `contentSchemaSQL` with correct columns, PK, FKs, and index.
- [x] `ContentSchemaVersion` is 3.
- [x] `Writer` interface includes `AddPredecessorRange(graph GraphID, bundleID, versionRange string) error`.
- [x] `ContentWriter.AddPredecessorRange` validates with `bsemver.ParseRange()` and uses plain `INSERT`.
- [x] `DeleteCatalogContent` deletes from `content_predecessor_ranges` in FK-safe order.
- [x] `querySuccessorsDirect` uses `fromID` for explicit edges, `fromVersion` for range evaluation, and deduplicates the union.
- [x] `querySuccessorsDescendant` does the same with the recursive CTE.
- [x] Range evaluation uses `blang/semver/v4` (`ParseRange`, `rng(version)`).
- [x] Range evaluation always runs regardless of version value (no special-casing of zero value).
- [x] Invalid ranges at query time yield an error in the iterator.

## Test Coverage

- [x] Writer tests: valid range, invalid range.
- [x] Query tests: range-only, explicit-only, union+dedup, `0.0.0` version, BundleID not in catalog, `||` syntax, composite graph.
- [x] Existing explicit-edge tests pass with updated call sites (regression).

## Project Conventions

- [x] `make ci` passes (lint, test, build).
- [x] No `//nolint` comments added.
- [x] Pure functions, no methods coupling logic to data (per `specs/mission.md`).
- [x] blang/semver appears in the public `Successors` signature (`bsemver.Version`) and in `catalog/v1/internal/` for range evaluation.
- [x] No unnecessary public API surface added beyond `AddPredecessorRange` on `Writer`.
- [x] Commit messages follow conventional commits format.
