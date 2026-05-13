---
status: done
pr: https://github.com/joelanford/library-olm/pull/12
---
# Deprecation Read

## Summary

Expose deprecation messages from the normalized content tables to consumers of
the catalog query API. The `Bundle` and `UpdateGraph` interfaces are unchanged.
Instead, when a bundle or graph is deprecated, the returned concrete type
additionally implements a `Deprecated` interface that callers can type-assert
against.

## Design

### Deprecated interface

A `Deprecated` interface in `catalog/v1/catalog.go`:

```go
type Deprecated interface {
    DeprecationMessage() string
}
```

Callers discover deprecation via type assertion:

```go
graph, _ := catalog.GetPackage(ctx, "my-pkg")
if d, ok := graph.(catalogv1.Deprecated); ok {
    fmt.Println("deprecated:", d.DeprecationMessage())
}
```

### deprecation struct

An unexported `deprecation` struct in `catalog/v1/sqlite/` implements
`Deprecated` once. All wrapper types embed it:

```go
type deprecation struct{ message string }
func (d deprecation) DeprecationMessage() string { return d.message }
```

### Wrapper types

When a graph or bundle has a non-NULL `deprecation_message`, the returned
value is wrapped in a type that embeds the original interface and
`deprecation`. Non-deprecated entities are returned unwrapped and do not
satisfy `Deprecated`.

**Graph wrappers** (in `catalog/v1/sqlite/query.go`):

```go
type deprecatedUpdateGraph struct {
    catalogv1.UpdateGraph
    deprecation
}

type deprecatedCompositeUpdateGraph struct {
    catalogv1.CompositeUpdateGraph
    deprecation
}
```

**Bundle wrapper** (in `catalog/v1/sqlite/query.go`):

```go
type deprecatedBundle struct {
    bundlev1.Bundle
    deprecation
}
```

All wrapper types and the `deprecation` struct live in `catalog/v1/sqlite`,
so no export gymnastics are needed.

### Where wrapping happens

**Graphs:** `queryGraphNodes` in `query.go` scans `g.deprecation_message`.
When non-NULL, wraps the constructed `*graphQuery` or
`*compositeGraphQuery` with the `deprecation` struct before yielding.

**Bundles:** `streamBundleRows` in `query.go` scans an additional
`deprecation_message` column. When non-NULL, wraps the `bundleRow` in
`deprecatedBundle` before yielding. All bundle query SQL is updated to
also SELECT `b.deprecation_message`.

### Interface preservation

- `deprecatedCompositeUpdateGraph` embeds `catalogv1.CompositeUpdateGraph`,
  so it satisfies both `UpdateGraph` and `CompositeUpdateGraph`
- `deprecatedBundle` embeds `bundlev1.Bundle`, so it satisfies `Bundle`
- The wrapping is invisible to callers who don't check for `Deprecated`
