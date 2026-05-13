# Verification

## Implementation Correctness
- [ ] `bundlev1.Release` type internally wraps `[]semver.PRVersion` from `blang/semver/v4` and delegates identifier comparison to `PRVersion.Compare`.
- [ ] `bundlev1.Release.Compare` ordering: empty sorts lower than non-empty; non-empty identifiers sort numeric-by-value, alphanumeric-lexically, numeric-before-alphanumeric, fewer-before-more.
- [ ] GoDoc for `bundlev1.Release` describes sorting rules in its own terms — no mention of semver pre-release.
- [ ] `bundlev1.VersionRelease` is a struct with `Version semver.Version` and `Release Release` fields, plus `Compare` (version first, then release).
- [ ] `bundlev1.NameVersionRelease` struct has `Name`, `Version`, and `Release` fields, plus `VersionRelease()` convenience method and `Compare` (name first, then version, then release).
- [ ] `bundlev1.Bundle` is an interface with `Name() string` and `VersionRelease() VersionRelease` methods.
- [ ] `catalogv1.UpdateGraph`, `catalogv1.CompositeUpdateGraph`, and `catalogv1.Catalog` interfaces compile and are consistent (method signatures, return types).
- [ ] `catalogv1.CompositeUpdateGraph` embeds `catalogv1.UpdateGraph`.
- [ ] All list methods return `iter.Seq2[T, error]`.

## Project Conventions
- [ ] No FBC-specific types or terminology appear in the public API.
- [ ] No dependencies on operator-framework/api or operator-framework/operator-registry.
- [ ] No cluster dependencies (no kubeconfig, no kube client, no controller-runtime).
- [ ] Public API surface is minimal — implementation details are in `internal/` packages where applicable.
- [ ] `make ci` passes.
