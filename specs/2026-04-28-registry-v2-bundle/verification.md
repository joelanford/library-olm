# Verification

## Implementation Correctness

- [ ] `bundle.yaml` parsing validates format, required fields, and produces correct `Metadata` values
- [ ] `FromFS` validates `resources/` directory exists with `main.jsonnet`
- [ ] `FromFS` loads `config.schema.json` as raw JSON and validates it is structurally valid JSON Schema
- [ ] `FromFS` returns clear errors for missing `bundle.yaml`, invalid format, missing `resources/main.jsonnet`
- [ ] `Render` validates user-provided config against JSON Schema when schema is present
- [ ] `Render` rejects invalid config with a descriptive error
- [ ] `Render` accepts any config (or nil) when no schema is present
- [ ] `Render` passes `bundle` and `config` as TLA via `TLACode`
- [ ] `Render` evaluates `main.jsonnet` via `EvaluateFile` so imports resolve relative to the file
- [ ] `Render` returns raw JSON output (a ClusterObjectSet)
- [ ] Standard `olm.libsonnet` produces a valid ClusterObjectSet with phases, lifecycle state, and collision protection
- [ ] `olm.phase` collects probes from resource descriptors and lifts them to `spec.progressionProbes`
- [ ] Standard `k8s-common.libsonnet` probe builders produce valid COS structured assertions (ConditionEqual, FieldsEqual)
- [ ] Probe selectors use GroupKind matching
- [ ] Phase modules receive bundle metadata directly (not wrapped in an olm object) — accessed as `bundle.name`, `bundle.version`, `bundle.release`
- [ ] Defaults merge via `std.mergePatch` in the Jsonnet entrypoint, not in the Go renderer
- [ ] Static manifests import correctly via `std.parseYaml(importstr '...')`
- [ ] End-to-end test: example bundle renders to a valid ClusterObjectSet with 5 phases and 2 progression probes
- [ ] `BundleAdapter` satisfies `bundlev1.Bundle` interface
- [ ] `BundleAdapter.ID()` returns deterministic identifier from name/version/release
- [ ] `BundleAdapter.NameVersionRelease()` returns correct identity values
- [ ] `BundleAdapter.Property()` returns `nil, nil` for any key

## Project Conventions

- [ ] All new public types and functions are in `bundle/registry/v2/`
- [ ] Implementation details are in `internal/` packages
- [ ] Types are pure data; transformations are standalone functions (not methods) — per `specs/mission.md`
- [ ] `FromFS` follows the `From` naming convention — per `specs/mission.md`
- [ ] No cluster dependencies (no kubeconfig, no kube client, no controller-runtime) — per `specs/mission.md`
- [ ] Legacy dependencies (`operator-framework/api`, `operator-framework/operator-registry`) are not used — per `specs/mission.md`
- [ ] New code has at least 70% statement coverage — per `specs/conventions.md`
- [ ] Overall project coverage does not decrease
- [ ] All builds use the `containers_image_openpgp` build tag — per `specs/tech-stack.md`
- [ ] `make ci` passes (lint, verify, test, build)
- [ ] No `//nolint` comments added without explicit permission
- [ ] Commits use conventional commit format, one logical change per commit
