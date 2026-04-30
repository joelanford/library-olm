# Requirements

- Define a `bundlev1.Release` type: a dot-separated sequence of identifiers with defined sorting (empty < non-empty; numeric identifiers by value; alphanumeric identifiers lexically; numeric before alphanumeric; fewer identifiers before more). Internally, `Release` wraps `[]semver.PRVersion` to delegate parsing and comparison.
- Define a `bundlev1.VersionRelease` struct pairing a `semver.Version` (from `blang/semver/v4`) with a `Release`.
- Define a `bundlev1.Bundle` interface with `Name() string` and `VersionRelease() VersionRelease` methods. Different catalog formats (registry+v1, Helm, registry+v2) provide their own implementations.
- Define a `bundlev1.NameVersionRelease` struct as the simplest `Bundle` implementation — just identity fields, no format-specific data.
- Define a `catalogv1.UpdateGraph` interface: a named collection of bundles with successor lookups for computing upgrade paths.
- Define a `catalogv1.CompositeUpdateGraph` interface: a `catalogv1.UpdateGraph` composed of named child `catalogv1.UpdateGraph`s, enabling catalog formats with channel-like concepts to expose them without leaking format-specific semantics into the core API.
- Define a `catalogv1.Catalog` interface with `ListPackages` and `GetPackage` methods that return `catalogv1.UpdateGraph`s.
- Use `iter.Seq2` for all list operations to support lazy iteration.
- No catalog format implementations in this work item — define the API surface only.
- No package-level metadata in the initial API — defer to a follow-up.

## Acceptance Criteria

- `bundle/v1/` package exports `bundlev1.Release` type, `bundlev1.VersionRelease` struct, `bundlev1.Bundle` interface, and `bundlev1.NameVersionRelease` struct with correct semantics.
- `catalog/v1/` package exports `catalogv1.UpdateGraph`, `catalogv1.CompositeUpdateGraph`, and `catalogv1.Catalog` interfaces.
- A caller can write the three patterns shown in the design (list bundles, query successors, channel-aware filtering with graceful fallback) against these interfaces.
- The API is format-agnostic — no FBC, channel, or OLM-specific terminology in the public types.
