# Implementation Plan

## 1. Introduce Result struct and migrate Resolve signature

**Files:** `resolver/v1/resolve.go`, `resolver/v1/resolve_test.go`

- Define `Result` struct with `Catalog`, `Package`, and `Bundles` fields
- Change `Resolve` return from `(catalogv1.Catalog, []bundlev1.Bundle, error)` to `(*Result, error)`
- Populate `Result.Package` from the `UpdateGraph` returned by `selectPackage`
- Update all existing tests to use the new return type (destructure `Result` fields)
- Verify all existing tests pass unchanged in logic

## 2. Add PreferNonDeprecatedBundles option

**Files:** `resolver/v1/resolve.go`

- Define `preferNonDeprecatedBundles` option type and `PreferNonDeprecatedBundles()` constructor
- Add `preferNonDeprecatedBundles bool` field to `resolveConfig`
- Adjust the final `slices.SortFunc` to use a two-tier comparison: deprecation
  status first (non-deprecated before deprecated), then version descending
  within each group

## 3. Extend test graph DSL for deprecation

**Files:** `resolver/v1/resolve_test.go`

- Add deprecation support to the test `graphImporter`:
  - `withGraphDeprecation(path []string, message string)` DSL option
  - `withBundleDeprecation(bundleID string, message string)` DSL option
- These call `w.SetGraphDeprecation` and `w.SetBundleDeprecation` in the
  importer's `Import` method

## 4. Add deprecation tests

**Files:** `resolver/v1/resolve_test.go`

- `TestResolve_DeprecatedPackage`: deprecated package graph satisfies
  `catalogv1.Deprecated` via type assertion on `Result.Package`
- `TestResolve_DeprecatedBundle`: deprecated bundles satisfy
  `catalogv1.Deprecated` via type assertion on `Result.Bundles` entries
- `TestResolve_DeprecatedChannel`: walk `Result.Package` as
  `CompositeUpdateGraph`, call `GetGraph`, type-assert child for `Deprecated`
- `TestResolve_NonDeprecated`: non-deprecated package and bundles do not
  satisfy `catalogv1.Deprecated`
- `TestResolve_PreferNonDeprecatedBundles`: non-deprecated bundles sort
  before deprecated bundles, version-descending within each group
