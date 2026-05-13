# Implementation Plan

1. Define `bundlev1.Release` type in `bundle/v1/` wrapping `[]semver.PRVersion` (from `blang/semver/v4`). Include `Compare` (delegates to `PRVersion.Compare` per identifier, with the override that empty sorts lower than non-empty), string parsing (via `semver.NewPRVersion` per identifier), and `fmt.Stringer`.
2. Define `bundlev1.VersionRelease` struct with `Version semver.Version` and `Release Release` fields, plus `Compare` (version first, then release).
3. Define `bundlev1.NameVersionRelease` struct with `Name string`, `Version semver.Version`, and `Release Release` fields, plus `VersionRelease()` convenience method and `Compare` (name first, then version, then release).
4. Define `bundlev1.Bundle` interface in `bundle/v1/` with `Name() string` and `VersionRelease() VersionRelease` methods.
5. Define `catalogv1.UpdateGraph` interface in `catalog/v1/`.
6. Define `catalogv1.CompositeUpdateGraph` interface in `catalog/v1/`.
7. Define `catalogv1.Catalog` interface in `catalog/v1/`.
