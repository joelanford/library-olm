# Verification

## Implementation Correctness

- [ ] `BundleIdentity` interface defined in `bundle/v1` with `ID()` and `NameVersionRelease()`.
- [ ] `Bundle` embeds `BundleIdentity`.
- [ ] `UpdateGraph.Successors` accepts `BundleIdentity` instead of separate ID and version.
- [ ] `CompositeUpdateGraph.Successors` accepts `BundleIdentity`.
- [ ] Internal query layer unpacks ID for explicit edges and version for predecessor ranges.
- [ ] `GetGraph` returns `CompositeUpdateGraph` when child has sub-graphs (single SQL query).
- [ ] `internal/util/test.BundleIdentity` struct is exported with `BundleID` and `NVR` fields.
- [ ] `NewBundleIdentity` derives ID as `{name}.v{version}[-{release}]`.
- [ ] No duplicate `testBundleIdentity` types across test packages.
- [ ] `importas` regex rule enforces `<name>util` alias for `internal/util/<name>` packages.

## Test Coverage

- [ ] All existing `Successors` tests pass with the new signature.
- [ ] Depth-2 graph path walking works via `GetGraph` returning `CompositeUpdateGraph`.
- [ ] `resolver/v1` tests use shared `testutil.NewBundleIdentity` and `testutil.BundleIdentity`.

## Project Conventions

- [ ] `make ci` passes (lint, test, build).
- [ ] No `//nolint` comments added.
- [ ] `specs/conventions.md` documents `internal/util/test` for cross-package test helpers.
- [ ] `specs/tech-stack.md` project structure includes `internal/util/test` and `internal/util/iterx`.
- [ ] Commit messages follow conventional commits format.
