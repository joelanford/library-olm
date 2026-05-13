# Verification

## Implementation Correctness
- [ ] `labelCatalogName` constant is defined (unexported) in `catalog/v1/`
- [ ] `Set()` injects the reserved label on every call (new and existing catalogs)
- [ ] `Set()` returns an error when `WithLabels()` contains a conflicting value for the reserved key
- [ ] `Set()` succeeds when `WithLabels()` contains a matching value for the reserved key
- [ ] The reserved label is preserved when `Set()` is called without `WithLabels()`
- [ ] No catalog in the store can exist without the reserved label after any `Set()` call
- [ ] All new and updated tests pass: `make test`

## Project Conventions
- [ ] No `//nolint` comments added
- [ ] Constant follows Go naming conventions (unexported, camelCase)
- [ ] Error messages are lowercase, descriptive, and don't end with punctuation
- [ ] One logical change per commit
- [ ] `make ci` passes (lint + test + build)
