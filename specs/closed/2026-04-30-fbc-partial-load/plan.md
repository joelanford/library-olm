# Implementation Plan

## 1. Add PackageError type

Create `catalog/fbc/error.go` with the `PackageError` struct, `Error()` method, and `Unwrap() []error` method.

## 2. Change Ingest to collect per-package errors

Modify `catalog/fbc/internal/ingest.go`:
- Change the `Ingest` return type to `(map[string][]error, error)` — the map holds per-package errors, the error is for fatal failures.
- In `metaToInsert` (and its callers in the walk callback), when a channel or bundle blob fails to parse or extract version info, record the error against `meta.Package` (or `ch.Package` / `b.Package` depending on the schema) and return `nil` to the walker.
- Package blob parse failures remain fatal (return error to walker).
- DB/writer errors remain fatal.

## 3. Change Normalize to skip failed packages and collect errors

Modify `catalog/fbc/internal/normalize.go`:
- Change `Normalize` signature to accept a `skipPackages map[string]bool` parameter and return `map[string][]error`.
- Skip any package in `skipPackages`.
- On `handler.Normalize` error: rollback, record the error against the package name, continue.
- On success: commit, continue.
- Return the collected errors map.

## 4. Update FromFS to merge errors and return partial catalog

Modify `catalog/fbc/catalog.go`:
- After `Ingest`, collect its per-package errors (don't close DB on non-fatal errors).
- Pass the failed package set to `Normalize`.
- After `Normalize`, collect its per-package errors.
- Merge both maps into `[]PackageError` values (one per failed package, combining errors from both phases).
- Join via `errors.Join` and return `(catalog, joinedErr)`.
- Only return `(nil, err)` for fatal errors from `Ingest` (its second return value).

## 5. Clean up failed package data

After normalization, delete raw table rows for failed packages so they don't appear in any queries. Add a helper in `internal/` that runs `DELETE FROM raw_olm_package WHERE name IN (...)` (and corresponding channel/bundle/entry rows). Call it from `FromFS` after merging errors.

## 6. Update existing tests

Modify `catalog/fbc/catalog_test.go`:
- Tests for single-package catalogs with bad data (`TestFromFS_InvalidSkipRange`, `TestFromFS_InvalidBundleVersion`, `TestFromFS_InvalidBundleRelease`, `TestFromFS_MissingPackageProperty`, `TestFromFS_MissingBundle`) should still assert an error is returned, but now also assert `cat != nil` and that the error is a `PackageError` with the expected package name.
- Add a multi-package test: one valid package + one malformed package. Assert the catalog contains only the valid package and the error contains a `PackageError` for the malformed one.
- Add a test: all packages malformed. Assert `cat != nil`, `ListPackages` yields zero, error contains `PackageError` for each.

## 7. Update example program

Modify `examples/query_operatorhubio/main.go`:
- After `fbc.FromFS`, check for error. If non-nil but catalog is also non-nil, log each `PackageError` as a warning and continue.
- Only fatal-exit if catalog is nil.
