---
status: done
pr: https://github.com/joelanford/library-olm/pull/4
---
# Bundle Identity

## Summary

Extract a `BundleIdentity` interface from `Bundle` and simplify the `Successors` method signature
across the catalog v1 API.

## Design

### BundleIdentity interface

`BundleIdentity` captures the identifying fields of a bundle without the content URI:

```go
type BundleIdentity interface {
    ID() BundleID
    NameVersionRelease() NameVersionRelease
}
```

`Bundle` embeds `BundleIdentity` and adds `URI()`.

### Successors signature

`UpdateGraph.Successors` changes from two separate parameters:

```go
Successors(ctx context.Context, fromID BundleID, fromVersion bsemver.Version) iter.Seq2[Bundle, error]
```

to a single `BundleIdentity`:

```go
Successors(ctx context.Context, from BundleIdentity) iter.Seq2[Bundle, error]
```

Callers who already have a `Bundle` (the common case) pass it directly instead of extracting
fields. The internal query layer unpacks `from.ID()` and `from.NameVersionRelease().Version`
for the explicit-edge and predecessor-range queries respectively.

### GetGraph depth support

`CompositeUpdateGraph.GetGraph` now checks whether the returned graph has child sub-graphs
(via a single SQL query) and returns a `CompositeUpdateGraph` when children exist. This enables
`WithGraphs` path walking at arbitrary depth.

### Shared test utility

`internal/util/test` provides `BundleIdentity` (exported struct) and `NewBundleIdentity(t, name, version, release)`
for constructing test identities with a derived ID of `{name}.v{version}[-{release}]`.
