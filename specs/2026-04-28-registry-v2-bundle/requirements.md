# Requirements

## Bundle Format

- A v2 bundle is a directory containing `bundle.yaml` at the root.
- `bundle.yaml` declares format (`registry+v2`), package name, semantic version, and optional release.
- Bundles may contain a `config.schema.json` file defining JSON Schema for user-provided config overrides.
- Bundles contain a `resources/` directory with a Jsonnet entrypoint (`main.jsonnet`) and supporting files.
- The Jsonnet entrypoint is a top-level argument function receiving `bundle` (metadata) and `config` (user overrides).
- The Jsonnet output is a `ClusterObjectSet` — no intermediate custom types.

## Parsing (FromFS)

- Parse `bundle.yaml` and validate required fields and format version.
- Load `config.schema.json` as raw JSON Schema if present.
- Validate that `resources/` directory exists and contains `main.jsonnet`.
- Store `resources/` as a sub-filesystem for later Jsonnet evaluation.
- Return a `Bundle` value containing metadata, config schema, and resources filesystem.
- Return clear errors for: missing `bundle.yaml`, invalid format, missing `resources/main.jsonnet`.

## Rendering (Render)

- Validate user-provided config against `config.schema.json` when the schema is present. The schema validates overrides only, not the merged result.
- Evaluate `main.jsonnet` with `bundle` and `config` passed as top-level arguments via `TLACode`.
- Return the raw JSON output (a `ClusterObjectSet`).
- Return clear errors for: schema validation failure, Jsonnet evaluation failure.

## Standard Jsonnet Helpers

- The library ships `olm.libsonnet` providing:
  - `clusterObjectSet(config, phaseResults)` — assembles a COS with phases, probes, lifecycle state, and collision protection.
  - `phase(name, resourcesFn, config)` — evaluates a phase module, returns `{phase, probes}`. Probes from resources are lifted to `spec.progressionProbes`.
- The library ships `k8s-common.libsonnet` providing probe builders matching the COS structured assertion types (ConditionEqual, FieldsEqual) and common Kubernetes resource helpers (RBAC, Deployment, Service, labels).
- Helpers are placed in `resources/vendor/` by convention. Imports resolve relative to the importing file — no load path configuration needed.

## Catalog Integration

- Provide a `BundleAdapter` that wraps a `Bundle` to satisfy the `bundlev1.Bundle` interface.
- `ID()` returns a deterministic bundle identifier derived from name, version, and release.
- `NameVersionRelease()` returns values from bundle metadata.
- `URI()` returns an empty string by default (set by the caller or image handler).
- `Property()` returns `nil, nil` for all keys (properties can be extended later).

## Acceptance Criteria

- `FromFS` correctly parses a well-formed v2 bundle directory and returns a `Bundle` with metadata, config schema, and resources filesystem.
- `FromFS` returns descriptive errors for malformed bundles (missing bundle.yaml, bad format, missing resources/main.jsonnet).
- `Render` evaluates Jsonnet with TLA and produces a valid ClusterObjectSet JSON output.
- `Render` validates config against JSON Schema and rejects invalid config with a clear error.
- Standard Jsonnet helpers produce a valid ClusterObjectSet with phases, inline objects, and progression probes.
- `BundleAdapter` satisfies `bundlev1.Bundle` and returns correct identity values.
- All new code has at least 70% statement coverage.
