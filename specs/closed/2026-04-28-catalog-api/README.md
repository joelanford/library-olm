---
status: done
---
# Catalog Public API

## Summary

Define a format-agnostic public API for catalogs built around two core abstractions: `catalogv1.UpdateGraph` (a named collection of bundles with upgrade relationships) and `catalogv1.Catalog` (a queryable collection of packages, each represented as a `catalogv1.UpdateGraph`). FBC-specific concepts like channels are handled through a `catalogv1.CompositeUpdateGraph` capability interface, keeping the core API clean while supporting real-world caller requirements like channel-based filtering.

## Design

### Bundle identity (`bundle/v1/`)

```go
// Release is a dot-separated sequence of identifiers that qualifies
// a bundle version (e.g. "rc1", "beta.2", "1.pre.0").
//
// Sorting rules:
//   - An empty Release sorts lower than any non-empty Release.
//   - Identifiers are compared left-to-right.
//   - A purely numeric identifier compares by value (2 < 10).
//   - An alphanumeric identifier compares lexically ("beta" < "rc").
//   - A numeric identifier sorts before an alphanumeric identifier.
//   - When all preceding identifiers are equal, fewer identifiers
//     sort before more (e.g. "rc" < "rc.1").
type Release struct { ... }

func (r Release) Compare(other Release) int { ... }

// VersionRelease pairs a semver version with a release qualifier.
type VersionRelease struct {
    Version semver.Version
    Release Release
}

func (vr VersionRelease) Compare(other VersionRelease) int { ... }

// NameVersionRelease is a bundle identity: name + version + release.
type NameVersionRelease struct {
    Name    string
    Version semver.Version
    Release Release
}

func (nvr NameVersionRelease) VersionRelease() VersionRelease { ... }
func (nvr NameVersionRelease) Compare(other NameVersionRelease) int { ... }

// Bundle represents a versioned unit of content in a catalog.
// Different catalog formats (registry+v1, Helm, registry+v2) provide
// their own implementations.
type Bundle interface {
    Name() string
    VersionRelease() VersionRelease
}
```

### Core interfaces (`catalog/v1/`)

```go
// UpdateGraph is a named collection of bundles with upgrade relationships.
// It is the fundamental query primitive: callers use it to list available
// bundles and to ask "what can I upgrade to from here?"
type UpdateGraph interface {
    Name() string
    ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error]
    Successors(ctx context.Context, from bundlev1.Bundle) iter.Seq2[bundlev1.Bundle, error]
}

// CompositeUpdateGraph is an UpdateGraph composed of named child UpdateGraphs.
// Catalog formats with channel-like concepts (e.g., FBC) implement this so
// callers can discover and query individual channels. Formats without channels
// return a plain UpdateGraph instead.
//
// ListBundles and Successors on a CompositeUpdateGraph operate across all
// child graphs (the union).
type CompositeUpdateGraph interface {
    UpdateGraph
    ListGraphs(ctx context.Context) iter.Seq2[UpdateGraph, error]
    GetGraph(ctx context.Context, name string) (UpdateGraph, error)
}

// Catalog is the top-level entry point for querying a catalog.
// Each package is represented as an UpdateGraph. Implementations backed
// by formats with channel concepts (e.g., FBC) return CompositeUpdateGraphs
// from these methods.
type Catalog interface {
    ListPackages(ctx context.Context) iter.Seq2[UpdateGraph, error]
    GetPackage(ctx context.Context, name string) (UpdateGraph, error)
}
```

### Caller patterns

**List all bundles in a package:**
```go
pkg, _ := catalog.GetPackage(ctx, "my-operator")
for bundle, err := range pkg.ListBundles(ctx) { ... }
```

**Find upgrade targets (no channel preference):**
```go
for next, err := range pkg.Successors(ctx, installed) { ... }
```

**Filter by channel (FBC):**
```go
if composite, ok := pkg.(catalogv1.CompositeUpdateGraph); ok {
    stable, err := composite.GetGraph(ctx, "stable")
    for next, err := range stable.Successors(ctx, installed) { ... }
} else {
    // "this catalog doesn't support channels"
}
```

**User asks for channel on a non-channel catalog:**
```go
if _, ok := pkg.(catalogv1.CompositeUpdateGraph); !ok {
    return fmt.Errorf("catalog does not support channel-based queries")
}
```

### How formats map to this model

The API is format-agnostic. Formats with channel-like concepts (e.g., FBC) return `catalogv1.CompositeUpdateGraph`s from `ListPackages`/`GetPackage`, where each child graph represents a channel. Simpler formats return plain `catalogv1.UpdateGraph`s; callers that attempt a `catalogv1.CompositeUpdateGraph` type assertion get `false` and can report that channels aren't supported.

See `specs/2026-04-29-catalog-fbc/` for the FBC-specific implementation.
