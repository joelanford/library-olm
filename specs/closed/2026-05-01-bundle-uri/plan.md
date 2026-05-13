# Implementation Plan

## 1. Expand `bundle/v1` types and interface

- Add `type BundleID string` to `bundle/v1/bundle.go`
- Change `Bundle` interface to: `ID() BundleID`, `NameVersionRelease() NameVersionRelease`, `URI() string`
- Remove `Name()` and `VersionRelease()` methods from `Bundle` interface
- Update `NameVersionRelease`: rename `BundleName` field to `Name`, add godoc documenting it as the package name, remove the `Name()` method. Update `Compare()` to reference `nvr.Name` instead of `nvr.BundleName`
- Update `bundle/v1/bundle_test.go`: update field references from `BundleName` to `Name`, remove any test asserting `NameVersionRelease` implements `Bundle`

## 2. Update catalog API signature

- In `catalog/v1/catalog.go`, change `Successors` in `UpdateGraph` from `from bundlev1.Bundle` to `from bundlev1.BundleID`
- This is a one-line change in the interface definition

## 3. Update FBC database schema

- In `catalog/fbc/internal/db.go`, add `image TEXT NOT NULL DEFAULT ''` column to `raw_olm_bundle` table
- Add `package_name TEXT NOT NULL DEFAULT ''` and `uri TEXT NOT NULL DEFAULT ''` columns to `bundles` normalized table

## 4. Update FBC ingest

- In `catalog/fbc/internal/ingest.go`, update `parseBundle` to:
  - Store `b.Image` as-is in the `image` column (no scheme prefix)
  - Include `image` in the `INSERT INTO raw_olm_bundle` statement

## 5. Update FBC handler

- In `catalog/fbc/internal/handler_olm_package.go`, update `insertBundles` to:
  - Select `image` from `raw_olm_bundle` alongside name, version, release
  - Prepend `"docker://"` to the image value to produce the URI (empty string if image is empty)
  - Include `package_name` (from the method's `packageName` parameter) and the scheme-prefixed `uri` in the `INSERT INTO bundles` statement

## 6. Update FBC query layer

- In `catalog/fbc/internal/query.go`:
  - Add an internal `bundleRow` type implementing `bundlev1.Bundle` (with `ID()`, `NameVersionRelease()`, `URI()` methods)
  - Update `yieldBundleRows` to select `id, package_name, version, release, uri` and construct `bundleRow` values
  - Update `Successors` methods on `CompositeUpdateGraphQuery` and `UpdateGraphQuery` to accept `bundlev1.BundleID`
  - Update `querySuccessorsDirect` and `querySuccessorsDescendant` to accept `bundlev1.BundleID` and use `string(from)` as the lookup key

## 7. Update compile-time checks and catalog.go

- In `catalog/fbc/catalog.go`:
  - Remove `var _ bundlev1.Bundle = bundlev1.NameVersionRelease{}`
  - Add `var _ bundlev1.Bundle = internal.BundleRow{}` (or equivalent, making the type exported from internal if needed for the check — alternatively place the check in the internal package itself)

## 8. Update example program

- In `examples/query_operatorhubio/main.go`:
  - Replace `b.Name()` with `b.ID()` or `b.NameVersionRelease().Name`
  - Replace `b.VersionRelease().Version` with `b.NameVersionRelease().Version`
  - Pass `b.ID()` to `Successors` instead of the full bundle
  - Optionally print `b.URI()` to demonstrate the new capability

## 9. Update tests

- In `catalog/fbc/catalog_test.go`:
  - Update all `bundlev1.NameVersionRelease{...}` used as `Successors` arguments to `bundlev1.BundleID("bundle-name")`
  - Update assertions that access `.Name()` or `.VersionRelease()` to use the new interface methods
  - Add test assertions for `URI()` on returned bundles
- In `bundle/v1/bundle_test.go`:
  - Remove `TestNameVersionRelease_Bundle` test (or update it to test NVR independently of Bundle interface)
- In `bundle/v1/example_test.go`:
  - Update if any examples reference `Bundle` interface through `NameVersionRelease`

## 10. Update FBC testdata (if needed)

- Review FBC test fixtures to ensure `olm.bundle` blobs include `image` fields so URI round-trip can be tested
