# Verification

## Implementation Correctness

- [ ] `Successors` signature on `UpdateGraph` and `CompositeUpdateGraph` is `Successors(ctx, id BundleID, version mmsemver.Version)`.
- [ ] All call sites updated for the new signature.
- [ ] `content_predecessor_constraints` table exists in `contentSchemaSQL` with correct columns, PK, FKs, and index.
- [ ] `ContentSchemaVersion` is 2.
- [ ] `Writer` interface includes `AddPredecessorConstraint(graph GraphID, bundleID, constraint string) error`.
- [ ] `ContentWriter.AddPredecessorConstraint` validates with `semver.NewConstraint()` and uses `INSERT OR REPLACE`.
- [ ] `DeleteCatalogContent` deletes from `content_predecessor_constraints` in FK-safe order.
- [ ] `querySuccessorsDirect` uses `id` for explicit edges, `version` for constraint evaluation, and deduplicates the union.
- [ ] `querySuccessorsDescendant` does the same with the recursive CTE.
- [ ] Constraint evaluation uses `Masterminds/semver/v3` (`NewConstraint`, `NewVersion`, `Check`).
- [ ] Constraint evaluation always runs regardless of version value (no special-casing of zero value).
- [ ] Invalid constraints at query time yield an error in the iterator.
- [ ] `Masterminds/semver/v3` is a direct dependency in `go.mod`.

## Test Coverage

- [ ] Writer tests: valid constraint, invalid constraint, replace semantics.
- [ ] Query tests: constraint-only, explicit-only, union+dedup, `0.0.0` version, BundleID not in catalog, `||` syntax, composite graph.
- [ ] Existing explicit-edge tests pass with updated call sites (regression).

## Project Conventions

- [ ] `make ci` passes (lint, test, build).
- [ ] No `//nolint` comments added.
- [ ] Pure functions, no methods coupling logic to data (per `specs/mission.md`).
- [ ] Masterminds/semver appears in the public `Successors` signature (`mmsemver.Version`) and in `catalog/v1/internal/` for constraint evaluation.
- [ ] No unnecessary public API surface added beyond `AddPredecessorConstraint` on `Writer`.
- [ ] Commit messages follow conventional commits format.
- [ ] New code has at least 70% statement coverage.
