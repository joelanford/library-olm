# Verification

## Implementation Correctness

- [x] `Validate` is exported and accepts `Bundle` by value
- [x] Delegates to the internal `registryv1.BundleValidator`
- [x] No new public types or configurable options exposed
- [x] `make build` passes

## Project Conventions

- [x] Function follows standalone-function pattern (not a method on Bundle)
- [x] Consistent with `FromFS`/`ToPlainManifests` value-receiver convention
- [x] Minimizes public API surface per `specs/mission.md`
- [x] No new dependencies introduced
