# Requirements

- `StoreReader` is a read-only subset of `Store` exposing `Get`, `List`, and `Select`.
- `Store` embeds `StoreReader` and adds `Set`, `Delete`, and `Close`.
- `Store.Select(labels.Selector) StoreReader` returns a read-only filtered view.
- `Resolve(ctx, reader StoreReader, packageName, ...ResolveOption) (Catalog, []bundlev1.Bundle, error)` is a standalone function that finds matching bundles and returns the selected catalog.
- `packageName` is a required positional argument.
- `WithGraphs(paths [][]string)` narrows to bundles in the sub-graphs at the given paths. Each path is a sequence of graph names walked from the package root. Paths that don't resolve are silently ignored. If none of the paths resolve, no bundles are returned.
- `WithMastermindsVersionConstraint(constraint mmsemver.Constraints)` narrows by a pre-parsed Masterminds/semver constraint.
- `WithSuccessorsOf(from bundlev1.BundleIdentity)` narrows to successors of the given bundle. When combined with `WithGraphs`, successors are scoped to the resolved sub-graphs only.
- Options compose as layered filters — each narrows the result set.
- Catalog priority: exactly one catalog provides results for a given package — the highest-priority catalog containing it. If multiple catalogs at the same priority have the package, return an ambiguity error.
- Returns `(Catalog, []bundlev1.Bundle, error)`: the selected catalog (nil when no catalog has the package) and bundles sorted by `VersionRelease` descending.
- Empty bundle slice (not error) when no bundles match the criteria; catalog is still returned to indicate where resolution happened.
- `labels.Selector` is from `k8s.io/apimachinery/pkg/labels`.
- `Masterminds/semver/v3` is promoted to a direct dependency for version constraint checking.

## Acceptance Criteria

- `Resolve(ctx, reader, "pkg")` with no options returns all bundles for "pkg" from the highest-priority catalog, sorted by version descending.
- `Resolve(ctx, reader, "pkg", WithGraphs([][]string{{"stable"}}))` returns only bundles in the "stable" sub-graph.
- `Resolve(ctx, reader, "pkg", WithGraphs([][]string{{"stable", "fast"}}))` walks "stable" then "fast" within it.
- `Resolve(ctx, reader, "pkg", WithGraphs([][]string{{"nonexistent"}}))` returns no bundles (path silently ignored).
- `Resolve(ctx, reader, "pkg", WithMastermindsVersionConstraint(constraint))` returns only bundles matching the constraint.
- `Resolve(ctx, reader, "pkg", WithSuccessorsOf(from))` returns successors of the given bundle.
- `Resolve(ctx, reader, "pkg", WithGraphs([][]string{{"stable"}}), WithSuccessorsOf(from))` returns successors scoped to the "stable" sub-graph.
- All three options combined narrows correctly (intersection).
- When "pkg" exists in two catalogs of equal priority, `Resolve` returns an ambiguity error.
- When "pkg" exists in catalogs of different priorities, only the highest-priority catalog's bundles are returned.
- `Resolve(ctx, store.Select(selector), ...)` only considers catalogs matching the selector.
- `Resolve(ctx, reader, "nonexistent-pkg")` returns no bundles, not an error.
