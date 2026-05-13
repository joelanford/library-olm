# Verification

## Implementation Correctness

- [ ] `bundlev1.BundleID` is defined as `type BundleID string`
- [ ] `bundlev1.Bundle` interface has exactly three methods: `ID() BundleID`, `NameVersionRelease() NameVersionRelease`, `URI() string`
- [ ] `bundlev1.NameVersionRelease` does not implement `bundlev1.Bundle` — no `ID()` or `URI()` methods exist on it
- [ ] `NameVersionRelease.BundleName` is renamed to `Name` with godoc documenting it as the package name
- [ ] `NameVersionRelease.Name()` method is removed
- [ ] `catalogv1.UpdateGraph.Successors` accepts `bundlev1.BundleID` (not `bundlev1.Bundle`)
- [ ] `raw_olm_bundle` table has `image` column (raw, no scheme prefix)
- [ ] `bundles` normalized table has `package_name` and `uri` columns
- [ ] FBC ingest stores raw `b.Image` in `raw_olm_bundle.image` (no scheme prefix)
- [ ] FBC handler prepends `docker://` during normalization and writes to `bundles.uri`
- [ ] FBC handler copies `package_name` to normalized `bundles` table
- [ ] FBC query returns bundles with `ID()`, `NameVersionRelease()`, and `URI()` populated
- [ ] `NameVersionRelease().Name` returns the package name (not the bundle ID)
- [ ] Successor queries use `BundleID` string directly as lookup key
- [ ] FBC test fixtures include `image` fields in `olm.bundle` blobs
- [ ] Tests verify `URI()` returns expected scheme-prefixed values
- [ ] Example program compiles and uses the new interface correctly

## Project Conventions

- [ ] Commits follow conventional commit format (`feat:`, `refactor:`, etc.)
- [ ] One logical change per commit
- [ ] Pure data types with standalone functions — no methods coupling logic to types (per `specs/mission.md`)
- [ ] From/To naming convention preserved where applicable
- [ ] Internal packages used for implementation details (`catalog/fbc/internal/`)
- [ ] No new legacy dependency usage introduced
- [ ] No cluster dependencies introduced
- [ ] `make ci` passes (lint, test, build)
- [ ] Test coverage does not decrease; new code has >= 70% statement coverage
