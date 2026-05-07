# Verification

## Implementation Correctness

- [ ] `StoreReader` interface includes `Get`, `List`, and `Select(labels.Selector) StoreReader`.
- [ ] `Store` embeds `StoreReader` and adds `Set`, `Delete`, and `Close`.
- [ ] `Select` returns a `StoreReader` (compile-time read-only guarantee).
- [ ] `Resolve` is a standalone function taking `StoreReader`, not a method on the interface.
- [ ] `Resolve` returns `(Catalog, []bundlev1.Bundle, error)` — catalog is nil only when no catalog has the package.
- [ ] `Select` filters `List` and `Get` to matching catalogs.
- [ ] `Resolve` finds the package in exactly one catalog (the highest-priority one containing it).
- [ ] `Resolve` returns an ambiguity error when the package exists in multiple catalogs of equal priority.
- [ ] `Resolve` returns no bundles (not an error) for a nonexistent package.
- [ ] `WithGraphs` walks each path from the package root; silently ignores paths that don't resolve; returns no bundles when no paths resolve.
- [ ] `WithMastermindsVersionConstraint` accepts a pre-parsed `mmsemver.Constraints` and filters correctly.
- [ ] `WithSuccessorsOf` calls `Successors(ctx, from)` on the appropriate graph(s).
- [ ] `WithGraphs` + `WithSuccessorsOf` scopes successors to the named sub-graphs only.
- [ ] Options compose as layered filters (intersection).
- [ ] Results sorted by version descending.
- [ ] Blang-to-Masterminds version conversion uses direct field mapping.

## Test Coverage

- [ ] No-option resolve: all bundles, sorted.
- [ ] Graph path filtering: correct filter at depth 1 and deeper, unknown path ignored, all-unknown returns no bundles.
- [ ] Version constraint: correct filter with pre-parsed constraint.
- [ ] Successors: correct results.
- [ ] Combined options: intersection behavior.
- [ ] Priority ordering: highest wins, ambiguity error.
- [ ] Select + Resolve: label filtering respected.
- [ ] Nonexistent package: no bundles returned.

## Project Conventions

- [ ] `make ci` passes (lint, test, build).
- [ ] No `//nolint` comments added.
- [ ] Pure functions where possible (per `specs/mission.md`).
- [ ] No cluster dependencies beyond `k8s.io/apimachinery/pkg/labels`.
- [ ] Commit messages follow conventional commits format.
