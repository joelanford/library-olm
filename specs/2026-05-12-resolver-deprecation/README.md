---
status: in-progress
---
# Resolver Deprecation

## Summary

Surface deprecation information in the resolver so callers can discover
whether resolved packages, channels, and bundles are deprecated.

Today `Resolve` returns `(catalogv1.Catalog, []bundlev1.Bundle, error)` and
ignores deprecation entirely — callers cannot see whether returned bundles or
their containing graphs (packages/channels) are deprecated.

Depends on: `2026-05-11-deprecation-read` (the `catalogv1.Deprecated`
interface and SQLite wrapper types that carry deprecation messages on
`UpdateGraph` and `Bundle` values).

## Design

### Result struct

Replace the multi-return with a `Result` struct:

```go
type Result struct {
    Catalog catalogv1.Catalog
    Package catalogv1.UpdateGraph
    Bundles []bundlev1.Bundle
}
```

`Package` is the package-level `UpdateGraph` selected during resolution.
Callers type-assert it for `catalogv1.Deprecated` to check package-level
deprecation. If the package is a `CompositeUpdateGraph`, callers can walk
into child graphs (channels) via `GetGraph` and type-assert those for
deprecation too.

`Bundles` continues to carry `catalogv1.Deprecated` via the existing wrapper
types from `catalog/v1/sqlite` — no new wrapping needed.

When no matching package is found, `Resolve` returns `(nil, nil)`,
same semantics as today.

### PreferNonDeprecatedBundles option

**`PreferNonDeprecatedBundles()`** — in the final sort, non-deprecated
bundles come first (version-descending), then deprecated bundles
(version-descending). Without this option, sort order is unchanged
(version-descending regardless of deprecation status).

This is the only deprecation-aware option in this spec. Graph-level
filtering and ranking are deferred — their semantics are nuanced
(transitivity, install vs upgrade, graph-bundle attribution) and are
better addressed once real callers show what they need.
