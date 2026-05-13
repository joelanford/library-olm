# Implementation Plan

## 1. Promote dependencies

- Promote `k8s.io/apimachinery` and `Masterminds/semver/v3` from indirect to direct in `go.mod`.
- Run `make ci` to verify nothing breaks.

## 2. StoreReader interface and Select method

- Define `StoreReader` interface in `catalog/v1/store.go` with `Get`, `List`, and `Select`.
- Refactor `Store` to embed `StoreReader` and add `Set`, `Delete`, and `Close`.
- Add `Select(labels.Selector) StoreReader` to `StoreReader`.
- Implement on `*db` in `catalog/v1/db.go`: return a `selectedStore` wrapper that implements
  `StoreReader`, filtering `List` and `Get` by label selector.

## 3. Resolve method and options

- Create `resolver/v1/resolve.go` with standalone `Resolve(ctx, StoreReader, packageName, ...ResolveOption) (Catalog, []bundlev1.Bundle, error)` function.
- Define `ResolveOption` interface and option constructors in `resolver/v1/resolve.go`:
  - `WithGraphs(paths [][]string)`
  - `WithMastermindsVersionConstraint(constraint mmsemver.Constraints)`
  - `WithSuccessorsOf(from bundlev1.BundleIdentity)`
- Implement resolve logic in `resolver/v1/resolve.go`:
  1. List catalogs, group by priority descending.
  2. Find the highest-priority group containing the package. Error on ambiguity.
  3. Get the package graph.
  4. If `WithGraphs`, walk each path from the package root via `GetGraph`; silently skip paths that don't resolve.
  5. Collect bundles: if `WithSuccessorsOf` is set, use `Successors()`; otherwise `ListBundles()`.
  6. If `WithMastermindsVersionConstraint`, filter by constraint check (convert blang version to
     Masterminds version via direct field mapping). The constraint is already parsed.
  7. Sort by version descending, return.

## 4. Tests

Tests in `resolver/v1/resolve_test.go`:
- Resolve with no options returns all bundles sorted by version descending.
- Resolve with `WithGraphs` filters correctly; unknown sub-graphs silently ignored; all-unknown returns no bundles.
- Resolve with `WithMastermindsVersionConstraint` filters correctly; invalid constraint errors.
- Resolve with `WithSuccessorsOf` returns successors only.
- Combined options narrow correctly.
- Priority: highest-priority catalog wins; ambiguity error on equal priority.
- Select + Resolve respects label filtering.
- Resolve for nonexistent package returns no bundles.

## 5. Verify

- Run `make ci`.
- Check that `Masterminds/semver/v3` is a direct dependency.
- Check that `k8s.io/apimachinery` is a direct dependency.
