# Implementation Plan

## 1. Define Deprecated interface and deprecation struct

**Files:** `catalog/v1/catalog.go`, `catalog/v1/sqlite/query.go`

- Add `Deprecated` interface with `DeprecationMessage() string` to
  `catalog/v1/catalog.go`
- Add unexported `deprecation` struct with `message string` field and
  `DeprecationMessage() string` method to `catalog/v1/sqlite/query.go`
- Add wrapper types to `catalog/v1/sqlite/`:
  - `deprecatedUpdateGraph` embedding `catalogv1.UpdateGraph` + `deprecation`
  - `deprecatedCompositeUpdateGraph` embedding
    `catalogv1.CompositeUpdateGraph` + `deprecation`
  - `deprecatedBundle` embedding `bundlev1.Bundle` + `deprecation`

## 2. Add deprecation_message to graph queries

**Files:** `catalog/v1/sqlite/query.go`

- Update `queryGraphNodes` SQL to SELECT `g.deprecation_message`
- Update the row scan to include the deprecation message
- When non-NULL, wrap the constructed `*graphQuery` or
  `*compositeGraphQuery` with `deprecation{message: *msg}` before
  yielding

## 3. Add deprecation_message to bundle queries

**Files:** `catalog/v1/sqlite/query.go`

- Update all bundle query SQL to also SELECT `b.deprecation_message`:
  - `queryBundlesDirect`
  - `queryBundlesDescendant`
  - `querySuccessorsDirect` (both explicit and range queries)
  - `querySuccessorsDescendant` (both explicit and range queries)
- Update `streamBundleRows` to scan the additional `deprecation_message`
  column as `*string` and pass it to `parseBundleRow`
- Update `querySuccessorsStreaming` similarly — it has its own inline
  `rows.Scan` and `parseBundleRow` calls for both explicit and range
  queries; both paths need the additional column scan
  (note: in range queries, `b.deprecation_message` comes before
  `pc.version_range` in the SELECT and scan order)
- Update `parseBundleRow` to accept `deprecationMsg *string` and return
  `bundlev1.Bundle` (interface) instead of `bundleRow` (concrete); when
  non-nil, return `deprecatedBundle` wrapping the `bundleRow`

## 4. Add tests

**Files:** `catalog/v1/sqlite/` (test file)

- Extend `TestDeprecation_EndToEnd` with subtests that query through the
  public API:
  - `GetPackage` on deprecated package → satisfies `catalogv1.Deprecated`
  - `ListGraphs` on deprecated package → deprecated channel satisfies
    `Deprecated`, non-deprecated channel does not
  - `ListBundles` → deprecated bundle satisfies `Deprecated`
  - Non-deprecated package/bundle → does not satisfy `Deprecated`
  - Deprecated package graph → still satisfies `CompositeUpdateGraph`
