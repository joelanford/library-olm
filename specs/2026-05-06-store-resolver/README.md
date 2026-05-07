---
status: done
pr: https://github.com/joelanford/library-olm/pull/4
---
# Store Resolver

## Summary

Add `Select` and `Resolve` methods to the catalog v1 `Store` for filtering catalogs by label
and finding matching bundles across the store. This is the first resolution primitive in
library-olm, covering install and upgrade use cases without dependency resolution.

## Design

### StoreReader

`StoreReader` is the read-only subset of `Store`:

```go
type StoreReader interface {
	Get(name string) (Catalog, error)
	List() ([]Catalog, error)
	Select(selector labels.Selector) StoreReader
}
```

`Store` embeds `StoreReader` and adds `Set`, `Delete`, and `Close`.

### Select

```go
func (s *db) Select(selector labels.Selector) StoreReader
```

Returns a read-only view of the store containing only catalogs whose labels match the selector.
Uses `k8s.io/apimachinery/pkg/labels.Selector`. The returned `StoreReader` shares the underlying
database but filters `List` and `Get` to matching catalogs. Passing a selected `StoreReader` to
`Resolve` limits resolution to matching catalogs.

### Resolve

```go
func Resolve(ctx context.Context, reader StoreReader, packageName string, opts ...ResolveOption) (Catalog, []bundlev1.Bundle, error)
```

Standalone function that finds bundles matching the given criteria across all catalogs in the
reader, sorted by version descending. `packageName` is required; all other criteria are optional.
Being a standalone function (rather than a method on `StoreReader`) keeps the interface a pure
data-access contract while providing a canonical resolution implementation that all consumers share.

### Options

| Option | Effect |
|---|---|
| `WithGraphs(paths [][]string)` | Narrow to bundles in the sub-graphs at the given paths. Each path is a sequence of graph names walked from the package root (e.g., `{"stable"}` for depth-1, `{"stable", "fast"}` for depth-2). Paths that don't resolve are silently ignored. When combined with `WithSuccessorsOf`, successors are scoped to the resolved sub-graphs only. |
| `WithMastermindsVersionConstraint(constraint mmsemver.Constraints)` | Narrow to bundles whose version satisfies the pre-parsed Masterminds/semver constraint. |
| `WithSuccessorsOf(from bundlev1.BundleIdentity)` | Narrow to bundles that are successors of the given bundle (explicit edges by ID, predecessor ranges by version). See `specs/2026-05-07-bundle-identity/`. |

Options compose as layered filters — each one narrows the result set. A bundle must satisfy
all specified criteria.

### Catalog priority

Catalogs are considered in priority order (highest first). If the requested package is found in
two or more catalogs with the same priority, `Resolve` returns an error indicating ambiguity.
Exactly one catalog provides results for a given package — the highest-priority catalog
containing it. If multiple catalogs at the same priority have the package, `Resolve` returns
an ambiguity error.

### Return value

Returns `(Catalog, []bundlev1.Bundle, error)`. The `Catalog` identifies which catalog was
selected for the package (nil only when no catalog has the package). Bundles are sorted by
`VersionRelease` descending. Empty bundle slice (not an error) when nothing matches the criteria.

### Algorithm

1. List catalogs (respecting label selection), group by priority descending.
2. For the highest priority group, find catalogs that contain the package.
3. If more than one catalog in the same priority group has the package, return an ambiguity error.
4. Get the package's `UpdateGraph` (or `CompositeUpdateGraph`).
5. Determine which graphs to query:
   - If `WithGraphs` is set, walk each path from the package root via `GetGraph` at each level.
     Paths that don't resolve (non-existent name or non-composite graph at an intermediate step)
     are silently skipped. Query the resolved sub-graphs.
   - Otherwise, query the package-level graph (union of all sub-graphs).
6. Collect bundles from the selected graph(s).
7. Apply `WithSuccessorsOf` filter: call `Successors(ctx, from)` on the selected
   graph(s) instead of `ListBundles`.
8. Apply `WithMastermindsVersionConstraint` filter: keep only bundles where
   `constraint.Check(mmsemverVersion)` is true. The blang version from the bundle is converted
   to a Masterminds version for checking.
9. Sort by version descending, return.
