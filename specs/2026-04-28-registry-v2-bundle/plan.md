# Implementation Plan

## 1. Core types and bundle.yaml parsing

Define the public types in `bundle/registry/v2/`.

Types to define:
- `Metadata` — struct with `Format` (`string`), `Name` (`string`), `Version` (`bsemver.Version`), `Release` (`bundlev1.Release`) fields and YAML tags with custom unmarshaling for version/release
- `Bundle` — struct with `Metadata`, `ConfigSchema` (`json.RawMessage`), and `ResourcesFS` (`fs.FS`)

Implement `bundle.yaml` parsing: unmarshal YAML, validate `format == "registry+v2"`, validate required fields (`name`, `version`), parse version as `bsemver.Version`, parse release as `bundlev1.Release`.

Tests: parse valid bundle.yaml, reject missing/invalid format, reject missing name/version.

## 2. FromFS parser

Implement `FromFS(fs.FS) (*Bundle, error)`.

Steps:
1. Read and parse `bundle.yaml` (from task group 1).
2. Read `config.schema.json` if present — store as `json.RawMessage`, validate it's valid JSON Schema using `santhosh-tekuri/jsonschema/v6`.
3. Validate `resources/` directory exists and contains `main.jsonnet`.
4. Store `resources/` as a sub-filesystem on the `Bundle`.
5. Return assembled `Bundle`.

Tests: parse complete bundle directory, handle missing config schema, reject missing bundle.yaml, reject missing resources/, reject missing main.jsonnet.

## 3. Jsonnet evaluation and Render function

Add `github.com/google/go-jsonnet` dependency. Implement `Render(*Bundle, config map[string]any) ([]byte, error)`.

Steps:
1. If `ConfigSchema` is present, compile the JSON Schema and validate `config` against it. Return error on validation failure.
2. Create a Jsonnet VM. JSON-encode `config` and bundle metadata, pass both via `TLACode`.
3. Evaluate `main.jsonnet` via `EvaluateFile`.
4. Return the raw JSON output.

Tests: end-to-end render with config schema validation pass/fail, end-to-end render without schema, Jsonnet import resolution, TLA parameter passing.

## 4. Standard Jsonnet helper libraries

Create the standard helper Jsonnet files that the library ships.

Files:
- `olm.libsonnet` — `clusterObjectSet(config, phaseResults)` and `phase(name, resourcesFn, config)` helpers. Produces ClusterObjectSet with phases, progression probes, lifecycle state, and collision protection.
- `k8s-common.libsonnet` — probe builders matching COS structured assertion types (ConditionEqual, FieldsEqual with GroupKind selectors), plus resource helpers (RBAC, Deployment, Service, labels). Only include helpers used by the example bundle.

Decide how these are shipped (embedded via `embed`, copied by tooling, or distributed via jsonnet-bundler).

Tests: build the example bundle (widget-operator with namespace, policy, CRDs, RBAC, deploy phases), render it, verify the output is a valid ClusterObjectSet with correct phases, objects, and progression probes.

## 5. bundlev1.Bundle adapter

Implement `BundleAdapter` that wraps a `*Bundle` and satisfies `bundlev1.Bundle`.

- `ID()` — compute deterministic `BundleID` from `name/version/release`
- `NameVersionRelease()` — return `bundlev1.NameVersionRelease` from parsed metadata
- `URI()` — return a URI field set on the adapter (default empty, set by caller)
- `Property()` — return `nil, nil` for all keys

Tests: verify interface satisfaction, verify identity values, verify property behavior.
