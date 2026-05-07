# Implementation Plan

## 1. BundleIdentity interface

- Define `BundleIdentity` interface in `bundle/v1/bundle.go` with `ID()` and `NameVersionRelease()`.
- Refactor `Bundle` to embed `BundleIdentity` and add `URI()`.

## 2. Successors signature

- Update `UpdateGraph.Successors` in `catalog/v1/catalog.go` to accept `bundlev1.BundleIdentity`.
- Update `CompositeUpdateGraphQuery.Successors` and `UpdateGraphQuery.Successors` in `catalog/v1/internal/query.go` to unpack `from.ID()` and `from.NameVersionRelease().Version`.
- Update `compositeUpdateGraphWrapper.Successors` in `catalog/v1/db.go` to pass through.
- Update `WithSuccessorsOf` in `resolver/v1/resolve.go` to accept `BundleIdentity`.

## 3. GetGraph depth support

- Update `CompositeUpdateGraphQuery.GetGraph` in `catalog/v1/internal/query.go` to return `(id int64, hasChildren bool, err error)` via a single SQL query.
- Update `compositeUpdateGraphWrapper.GetGraph` in `catalog/v1/db.go` to return `CompositeUpdateGraph` when `hasChildren` is true.

## 4. Shared test utility

- Create `internal/util/test/bundle.go` with exported `BundleIdentity` struct and `NewBundleIdentity(t, name, version, release)`.
- Remove duplicate `testBundleIdentity` types from `catalog/v1/db_test.go`, `catalog/fbc/catalog_test.go`, and `resolver/v1/resolve_test.go`.
- Add `importas` regex rule for `internal/util/(\w+)` → `${1}util`.

## 5. Update call sites

- Update all `Successors` call sites in test files and examples to pass `BundleIdentity`.
- Update `specs/conventions.md` to document `internal/util/test` for cross-package test helpers.

## 6. Verify

- Run `make ci`.
- Confirm no duplicate test helper types remain.
