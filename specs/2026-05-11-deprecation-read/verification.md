# Verification

## Implementation Correctness

- [ ] `Deprecated` interface defined in `catalog/v1/catalog.go` with
      `DeprecationMessage() string`
- [ ] Unexported `deprecation` struct in `catalog/v1/sqlite/` implements
      the method once
- [ ] All three wrapper types embed the `deprecation` struct
- [ ] `Bundle`, `UpdateGraph`, `CompositeUpdateGraph` interfaces unchanged
- [ ] `queryGraphNodes` SQL selects `g.deprecation_message` and wraps
      when non-NULL
- [ ] Bundle query SQL selects `b.deprecation_message` in all four query
      functions (direct bundles, descendant bundles, direct successors,
      descendant successors)
- [ ] `streamBundleRows` scans deprecation message and wraps when non-NULL
- [ ] `querySuccessorsStreaming` scans deprecation message and wraps when
      non-NULL (both explicit and range paths)
- [ ] Deprecated composite graphs satisfy both `CompositeUpdateGraph` and
      `Deprecated`
- [ ] Non-deprecated entities do NOT satisfy `Deprecated`
- [ ] Test covers all three levels (package, channel, bundle)
- [ ] Test covers non-deprecated entities failing the type assertion
- [ ] Test covers `CompositeUpdateGraph` assertion on deprecated package

## Project Conventions

- [ ] `make ci` passes (lint + test + build)
- [ ] No `//nolint` comments added
- [ ] No changes to `Bundle`, `UpdateGraph`, or `CompositeUpdateGraph`
      interfaces
- [ ] No cluster dependencies introduced
- [ ] Existing tests pass without modification
