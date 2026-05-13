# Requirements

- `bundlev1.BundleID` is a `string` type representing the unique identifier for a bundle
- `bundlev1.Bundle` interface has three methods: `ID() BundleID`, `NameVersionRelease() NameVersionRelease`, `URI() string`
- `bundlev1.NameVersionRelease` field `BundleName` is renamed to `Name` (documented as the package name); `Name()` method is removed
- `NameVersionRelease` no longer implements `Bundle`
- `catalogv1.UpdateGraph.Successors` accepts `bundlev1.BundleID` instead of `bundlev1.Bundle`
- FBC ingest stores the raw image reference from `declcfg.Bundle.Image` (no scheme prefix) in `raw_olm_bundle.image`
- FBC handler validates the image field during normalization: must be non-empty and parse as a NamedTagged or Canonical docker reference
- FBC handler prepends the `docker://` scheme during normalization when writing to the `bundles.uri` column
- Bundles with missing or invalid image references cause a per-package error (soft fail)
- FBC normalized `bundles` table includes `package_name` and `uri` columns
- FBC query layer returns bundles implementing the full `Bundle` interface with URI populated
- FBC successor queries use `BundleID` directly as the lookup key

## Acceptance Criteria

- `make ci` passes (lint, test, build)
- All bundle queries from FBC catalogs return bundles with non-empty `URI()` (when `declcfg.Bundle.Image` is non-empty)
- `Successors` works with `BundleID` — existing successor tests pass with updated signatures
- `NameVersionRelease` has no `ID()` or `URI()` methods and does not implement `Bundle`
- The example program (`examples/query_operatorhubio/`) works with the new interface
- Compile-time interface checks verify `Bundle` satisfaction for FBC's concrete type
