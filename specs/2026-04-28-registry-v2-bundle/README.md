---
status: ready
---
# Registry+v2 Bundle Format

## Summary

Define a next-generation bundle format (registry+v2) that replaces the CSV-centric registry+v1 with a format supporting arbitrary Kubernetes resource types, ordered phases with progression probes, and Jsonnet-based templating. Bundles render directly to a `ClusterObjectSet` — the Kubernetes resource that operator-controller uses for phased rollout, probe-based progression, and lifecycle management.

## Design

### Directory Layout

```
bundle.yaml                    # identity and format declaration
config.schema.json             # JSON Schema for user-provided config overrides (optional)
resources/
  main.jsonnet                 # Jsonnet entrypoint, outputs a ClusterObjectSet
  vendor/                      # dependencies (e.g., OLM-provided helpers)
    olm.libsonnet
    k8s-common.libsonnet
  lib/                         # bundle-specific modules
    defaults.libsonnet
    namespace.libsonnet
    crds.libsonnet
    rbac.libsonnet
    deploy.libsonnet
  static/                      # pre-generated manifests (e.g., CRDs)
    widgets.crd.yaml
```

The renderer evaluates `resources/main.jsonnet` directly — it has zero knowledge of the bundle's internal directory structure. Imports resolve relative to the importing file.

### bundle.yaml

The root manifest declares bundle identity and format. Not processed through the template engine.

```yaml
format: registry+v2
name: widget-operator
version: 1.0.0
release: rc.1
```

| Field | Required | Description |
|---|---|---|
| `format` | yes | Must be `registry+v2` |
| `name` | yes | Package name |
| `version` | yes | Semantic version |
| `release` | no | Dot-separated release qualifiers (e.g., `rc.1`) |

### config.schema.json

Optional JSON Schema file validating user-provided config overrides. The schema governs what the user provides — not the merged result after defaults are applied.

### Jsonnet Rendering

Bundles use Jsonnet for templating. The entrypoint (`main.jsonnet`) is a top-level argument function receiving `bundle` (metadata from bundle.yaml) and `config` (user-provided overrides):

```jsonnet
function(bundle, config)
  local olm = (import 'vendor/olm.libsonnet')(bundle);
  local cfg = std.mergePatch((import 'lib/defaults.libsonnet')(bundle), config);

  olm.clusterObjectSet(cfg, [
    olm.phase('namespace', import 'lib/namespace.libsonnet', cfg),
    olm.phase('crds', import 'lib/crds.libsonnet', cfg),
    olm.phase('rbac', import 'lib/rbac.libsonnet', cfg),
    olm.phase('deploy', import 'lib/deploy.libsonnet', cfg),
  ])
```

Key design decisions:

- **Top-level arguments (TLA)** — `bundle` and `config` are passed via `TLACode`, not `ExtVar`. No globals. The bundle is a callable function that can be imported and tested.
- **Defaults in Jsonnet** — bundle authors define defaults in `defaults.libsonnet` and merge with user config via `std.mergePatch`. The renderer doesn't handle defaults or merging.
- **Config passed as JSON string** — the renderer JSON-encodes config and passes it via `TLACode`. JSON is valid Jsonnet code, so it evaluates directly to an object.
- **Phase modules** — each is a `function(bundle, config)` returning `[{resource, probe?}]`. They receive bundle metadata (name, version, release) and merged config. They import `k8s-common.libsonnet` for resource and probe helpers.
- **Static manifests** — pre-generated resources (e.g., CRDs) are imported via `std.parseYaml(importstr '...')`.

### ClusterObjectSet Output

The Jsonnet entrypoint outputs a `ClusterObjectSet` directly — the Kubernetes resource that operator-controller uses for phased rollout. No intermediate custom types (Bundle, BundleResource, etc.) are needed.

`olm.libsonnet` provides two helpers:

- `clusterObjectSet(config, phaseResults)` — assembles a COS with phases, progression probes, lifecycle state, and collision protection.
- `phase(name, resourcesFn, config)` — evaluates a phase module and returns `{phase, probes}`. Probes declared alongside resources are automatically lifted to `spec.progressionProbes`.

### Probes

Probes use the ClusterObjectSet `progressionProbes` API — structured assertions (ConditionEqual, FieldsEqual, FieldValue) with selectors (GroupKind, Label). Phase modules declare probes alongside resources; `olm.phase` collects them and `olm.clusterObjectSet` aggregates them at the spec level.

`k8s-common.libsonnet` provides probe builders matching the COS assertion types:

```jsonnet
probes:: {
  crdEstablished:: {
    selector: { type: 'GroupKind', groupKind: { group: 'apiextensions.k8s.io', kind: 'CustomResourceDefinition' } },
    assertions: [
      { type: 'ConditionEqual', conditionEqual: { type: 'Established', status: 'True' } },
    ],
  },
  deploymentReady:: {
    selector: { type: 'GroupKind', groupKind: { group: 'apps', kind: 'Deployment' } },
    assertions: [
      { type: 'ConditionEqual', conditionEqual: { type: 'Available', status: 'True' } },
      { type: 'FieldsEqual', fieldsEqual: { fieldA: 'spec.replicas', fieldB: 'status.updatedReplicas' } },
      { type: 'FieldsEqual', fieldsEqual: { fieldA: 'spec.replicas', fieldB: 'status.readyReplicas' } },
      { type: 'FieldsEqual', fieldsEqual: { fieldA: 'spec.replicas', fieldB: 'status.replicas' } },
    ],
  },
},
```

### Renderer

The renderer is minimal:

1. Parse `bundle.yaml` — extract name, version, release, validate format.
2. Validate user config against `config.schema.json` (if present).
3. Evaluate `main.jsonnet` via `EvaluateFile` with `bundle` and `config` as TLA.
4. Return the rendered ClusterObjectSet.

No custom Go types, no unwrapping. The Jsonnet output is the final artifact. Runtime concerns (revision numbering, service account annotations, cluster coordination) are handled by the caller.

### Public API

Package `bundle/registry/v2`:

```go
// FromFS parses a v2 bundle from a filesystem. Validates structure and
// loads the Jsonnet entrypoint. Does not render.
func FromFS(fsys fs.FS) (*Bundle, error)

// Render validates config against the bundle's JSON Schema (if present),
// evaluates the Jsonnet entrypoint, and returns the ClusterObjectSet JSON.
func Render(b *Bundle, config map[string]any) ([]byte, error)
```

Key types:
- `Bundle` — parsed but unrendered bundle (metadata + config schema + resources filesystem)
- `Metadata` — name, version, release, format

The rendered output is raw JSON/YAML — a `ClusterObjectSet` that can be applied directly. No custom output types needed.
