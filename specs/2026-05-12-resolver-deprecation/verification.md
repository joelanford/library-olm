# Verification

## Implementation Correctness

- [ ] `Result` struct has exactly three fields: `Catalog`, `Package`, `Bundles`
- [ ] `Resolve` returns `(*Result, error)`
- [ ] `Result.Package` is populated from `selectPackage`
- [ ] When no package found, returns `(nil, nil)`
- [ ] Deprecated package graph satisfies `catalogv1.Deprecated` on `Result.Package`
- [ ] Deprecated bundles satisfy `catalogv1.Deprecated` on `Result.Bundles` entries
- [ ] Non-deprecated values do not satisfy `catalogv1.Deprecated`
- [ ] Child graph deprecation reachable via `Result.Package` → `CompositeUpdateGraph.GetGraph` → type assertion
- [ ] `PreferNonDeprecatedBundles` sorts non-deprecated bundles before deprecated, version-descending within each group
- [ ] All existing tests pass after `Result` struct migration

## Project Conventions

- [ ] No new public types beyond `Result` and `PreferNonDeprecatedBundles`
- [ ] Pure functions — no methods coupling logic to data types
- [ ] No cluster dependencies introduced
- [ ] No new legacy dependency usage
- [ ] Tests use the existing test DSL pattern extended for deprecation
- [ ] Test coverage for new code ≥ 70%
- [ ] `make ci` passes (lint, test, build)
- [ ] Commit messages use conventional commits format
