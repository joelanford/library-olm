# Requirements

- `Resolve` returns `(*Result, error)` instead of a multi-return tuple
- `Result.Package` is the package-level `UpdateGraph` selected during resolution
- `Result.Bundles` contains the matched bundles as before
- `Result.Catalog` contains the selected catalog as before
- When no matching package is found, `Resolve` returns `(nil, nil)`
- Deprecation on the package graph and bundles is discoverable via type assertion to `catalogv1.Deprecated` (existing pattern)
- Callers can type-assert `Result.Package` to `CompositeUpdateGraph`, then use `GetGraph` to walk into child graphs and type-assert those for deprecation
- `PreferNonDeprecatedBundles()` option sorts deprecated bundles to the end of `Result.Bundles`, preserving version-descending order within each group (non-deprecated first, then deprecated)

## Acceptance Criteria

- All existing resolver tests continue to pass after the `Result` struct migration (updated call sites)
- Deprecated package graph satisfies `catalogv1.Deprecated` via type assertion on `Result.Package`
- Deprecated bundles satisfy `catalogv1.Deprecated` via type assertion on `Result.Bundles` entries
- Non-deprecated package and bundles do not satisfy `catalogv1.Deprecated`
- Child graph deprecation is reachable via `Result.Package` → `CompositeUpdateGraph.GetGraph` → type assertion
- `PreferNonDeprecatedBundles` test: non-deprecated bundles sort before deprecated bundles, version-descending within each group
- `make ci` passes
