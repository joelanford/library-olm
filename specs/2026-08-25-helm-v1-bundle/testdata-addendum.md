# Testdata Addendum

## Purpose

Simplify the `bundle/helm/v1` unit tests around one realistic, compact,
feature-dense, valid Helm chart stored at
`bundle/helm/v1/testdata/base-chart/`.

The base chart should cover nearly the entire successful `FromFS` and
`ToPlainManifests` pipeline in one scenario. Every renderer error case should
start from a freshly loaded base chart and mutate only the feature needed to
produce that error. This replaces the growing in-memory chart construction DSL
and makes each test's difference from valid behavior explicit.

This is not a golden-output or Helm CLI comparison. The fixture exists to give
the tests a canonical valid input with normal Helm directory structure,
defaults, schemas, and local application and library dependencies.

## Goals

- Exercise chart loading and rendering together on a realistic chart layout.
- Cover parent and dependency value defaults, parent-provided dependency
  overrides, and caller-provided overrides in one happy-path render.
- Validate both the parent and dependency `values.schema.json` files.
- Exercise Helm structures and cross-file relationships supported by this
  renderer in one understandable chart.
- Exercise caller-supplied Kubernetes capabilities, built-in objects, chart
  file access, named templates, NOTES rendering and exclusion, value flow,
  enabled and disabled dependencies, CRDs, and manifest ordering.
- Exercise the fixed upgrade release state without using any denied functions.
- Keep the base chart free of hooks and disallowed `helm.sh/*` metadata.
- Make renderer error tests small mutations of the same known-valid chart.
- Keep table rows shaped like the function under test: chart, release name,
  namespace, options, and an assertion function receiving objects and error.
- Preserve Tobari's ability to attribute distinct production paths to distinct
  subtests.

## Non-Goals

- Do not invoke the Helm CLI or compare output with generated golden files.
- Do not build a general-purpose chart fixture framework.
- Do not retain `yamlObject`, custom JSON marshaling, `templates`, or similar
  in-memory YAML construction helpers once the fixture replaces their uses.
- Do not exercise Go `text/template` syntax or allowed template functions
  exhaustively in the base chart. Use only enough template functionality to
  prove relationships between chart files, such as `include` and `template`
  consuming definitions from `_helpers.tpl`.
- Do not add template expressions solely to demonstrate isolated functions when
  they do not depend on another chart file, value source, built-in object, or
  chart relationship.
- Do not include unsupported Helm behavior such as hooks, cluster lookup,
  non-hermetic functions, or disallowed `helm.sh/*` metadata.
- Do not avoid normal Helm value substitution merely to simplify test fixture
  construction. The on-disk chart should template both string and integer
  values where real charts commonly do so, without trying to demonstrate every
  `text/template` construct.

## Base Chart Layout

Create the following files:

```text
bundle/helm/v1/testdata/base-chart/
  Chart.yaml
  values.yaml
  values.schema.json
  crds/examples.example.com.yaml
  files/message.txt
  templates/_helpers.tpl
  templates/configmap.yaml
  templates/service.yaml
  templates/deployment.yaml
  templates/NOTES.txt
  charts/child/
    Chart.yaml
    values.yaml
    values.schema.json
    templates/configmap.yaml
  charts/optional/
    Chart.yaml
    values.yaml
    templates/configmap.yaml
  charts/library/
    Chart.yaml
    templates/_helpers.tpl
```

The CRD is included so the same happy-path render also verifies the renderer's
fixed `IncludeCRDs` behavior. Keep it as small as Kubernetes CRD decoding
permits.

### Parent Chart Metadata

`Chart.yaml` should define an application chart named `base-chart`, version
`1.0.0`, with unpacked local dependencies:

```yaml
apiVersion: v2
name: base-chart
version: 1.0.0
appVersion: 2.0.0
dependencies:
- name: child
  version: 1.0.0
  condition: child.enabled
- name: optional
  version: 1.0.0
  condition: optional.enabled
- name: library
  version: 1.0.0
```

No repository is needed because all dependencies are already present under
`charts/`.

The `library` dependency is also unpacked under `charts/` and must be declared
with `type: library` in its own `Chart.yaml`. It should not render a Kubernetes
object on its own.

### Parent Values

`values.yaml` should distinguish three value sources:

```yaml
parent:
  defaultOnly: parent-default
  overridden: parent-default
  requiredValue: required-default
  replicas: 2
  servicePort: 8080
child:
  enabled: true
  parentOverride: parent-override
  userOverride: parent-default
optional:
  enabled: true
global:
  shared: global-default
```

The parent template should expose `parent.defaultOnly` and
`parent.overridden`. The enabled child template should expose its own default
plus `parentOverride`, `userOverride`, and the Helm `global` value after
coalescing. `WithValues` should set `optional.enabled` to `false`, overriding the
parent default and preventing the optional child's template from producing an
object.

### Parent Schema

`values.schema.json` should require the `parent` object and validate all parent
fixture values. `defaultOnly`, `overridden`, and `requiredValue` are strings;
`replicas` and `servicePort` are integers. Do not duplicate the child schema's
validation of child-owned values in the parent schema. This allows a focused
test to prove that a failure came from the child schema.

Use a normal JSON Schema object similar to:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["parent"],
  "properties": {
    "parent": {
      "type": "object",
      "required": [
        "defaultOnly",
        "overridden",
        "requiredValue",
        "replicas",
        "servicePort"
      ],
      "properties": {
        "defaultOnly": {"type": "string"},
        "overridden": {"type": "string"},
        "requiredValue": {"type": "string"},
        "replicas": {"type": "integer", "minimum": 0},
        "servicePort": {"type": "integer", "minimum": 1}
      }
    }
  }
}
```

Adjust the declared draft only if Helm's schema validator requires a different
draft identifier.

### Child Values And Schema

The child `Chart.yaml` should be a minimal v2 application chart named `child`
at version `1.0.0`.

The child `values.yaml` should contain:

```yaml
childDefaultOnly: child-default
parentOverride: child-default
userOverride: child-default
```

The child `values.schema.json` should require all three fields to be strings.
The successful render must demonstrate these final values:

- `childDefaultOnly` remains `child-default` from the child chart.
- `parentOverride` becomes `parent-override` from the parent `values.yaml`.
- `userOverride` becomes `user-override` from `WithValues`.
- `global.shared` is visible in both parent and child templates, with its final
  value supplied by `WithValues`.

### Optional Child

The `optional` chart should be a second minimal unpacked dependency with one
template that would render a uniquely named ConfigMap when enabled. Its parent
default is `optional.enabled: true`, while the happy-path `WithValues` input sets
it to `false`. Assert that the optional ConfigMap is absent.

This second dependency lets one render cover both sides of Helm dependency
enablement without sacrificing the enabled child's defaults, schema, parent
overrides, and rendered output. The optional chart does not need its own schema
or additional template logic.

### Library Dependency

The `library` chart should be a minimal v2 chart named `library` at version
`1.0.0`, with `type: library`. Its `templates/_helpers.tpl` should define one
named helper, such as `library.label`, that returns a stable string. The parent
ConfigMap should consume that helper with `include` and assert the rendered
value. The library chart must not produce a standalone manifest; this verifies
that library dependencies contribute templates while remaining non-renderable.

### Templates And Supported Features

The fixture should deliberately cover the supported Helm structural feature
surface below.
Prefer adding a field to an existing ConfigMap over adding another resource when
the feature only needs a rendered string assertion.

| Feature | Fixture coverage |
|---|---|
| Parent chart defaults | Read `parent.defaultOnly` from the parent `values.yaml`. |
| Caller value overrides | Override `parent.overridden`, `child.userOverride`, and `global.shared` through `WithValues`. |
| Parent-to-child overrides | Override the child's `parentOverride` under the parent `child` values key. |
| Child chart defaults | Read `childDefaultOnly` from the child `values.yaml`. |
| Global values | Read the final `global.shared` value from both parent and child templates. |
| Parent schema | Successfully validate the final parent values against the parent `values.schema.json`. |
| Child schema | Successfully validate final coalesced child values against the child `values.schema.json`. |
| Dependency conditions | Keep `child.enabled: true` so one unpacked dependency renders, and override `optional.enabled` to `false` through `WithValues` so the second dependency does not render. |
| Library dependency | Load an unpacked `type: library` dependency, consume its named helper from the parent template, and assert that it contributes no standalone object. |
| Release identity | Render `.Release.Name`, `.Release.Namespace`, and `.Release.Service`. |
| Fixed release state | Render `.Release.IsInstall`, `.Release.IsUpgrade`, and `.Release.Revision`. |
| Chart metadata | Render `.Chart.Name`, `.Chart.Version`, and `.Chart.AppVersion`; set `appVersion` in `Chart.yaml`. |
| Template metadata | Render `.Template.Name` and `.Template.BasePath`. |
| Subchart access | If supported by the pinned Helm SDK, read a child value through `.Subcharts.child` from the parent template in addition to rendering the child template normally. |
| Kubernetes version | Render `.Capabilities.KubeVersion.Version`, `.Major`, and `.Minor` from `WithKubeVersion`. |
| API versions | Test `.Capabilities.APIVersions.Has` for both a supplied group version and a supplied concrete resource kind. |
| Chart files | Add `files/message.txt` and render it with `.Files.Get`. Do not exercise every `.Files` helper. |
| Named templates | Define helpers in `_helpers.tpl` and consume them from another file through both `include` and the `template` action. |
| Required values | Successfully call `required` on a supplied value. |
| Non-string value templating | Template `parent.replicas` into Deployment `spec.replicas` and `parent.servicePort` into a Service port. Assert decoded values retain their integer type. |
| Multiple object kinds | Render at least a ConfigMap, Service, and Deployment to show arbitrary Kubernetes objects are decoded. |
| Install order | Assert the returned ConfigMap, Service, and Deployment follow Helm's install order. |
| CRDs | Include the CRD under `crds/` and assert it is returned. |
| NOTES rendering and exclusion | Make `templates/NOTES.txt` read a value and consume a helper from `_helpers.tpl`. Its successful evaluation is part of rendering, but its unique marker must not become a returned object. |
| Allowed Helm metadata | Put the allowlisted `helm.sh/chart` label on one rendered object and assert it remains present. |
| Non-Helm metadata | Include one ordinary application label or annotation and assert it remains present. |

Use `_helpers.tpl` for named template definitions consumed by the ConfigMap and
NOTES files. Put most scalar assertions in the parent ConfigMap `data` map, but
also template values into typed Kubernetes fields. Leave numeric template
actions unquoted in the fixture YAML and assert the decoded object contains the
expected numeric type.

Avoid isolated template-language demonstrations. A template action belongs in
the base chart when it proves a relationship with values, capabilities, chart
metadata, another file, or a dependency. Exhaustive allowed-function behavior
belongs in `internal/hermetic`, not in this fixture.

The Service and Deployment should remain structurally minimal. Their purpose is
to cover arbitrary object decoding and install ordering, not to model an actual
workload. The child ConfigMap should expose `childDefaultOnly`,
`parentOverride`, `userOverride`, and `global.shared`.

If a supported feature cannot be included without making the fixture brittle,
document the reason in the implementation diff and keep a focused happy-path
test for that feature. The default decision should be to include supported
features in `base-chart`, not to create another standalone success test.

The fixture must not contain:

- Helm hooks.
- Denied random, time, crypto, DNS, or `lookup` functions.
- Non-allowlisted `helm.sh/*` annotations or labels.

An allowlisted `helm.sh/chart` label may be included to exercise the positive
metadata path.

## Test Helpers

Add one helper that always loads a fresh fixture:

```go
func baseChart(t *testing.T, mutate ...func(*Chart)) *Chart {
    t.Helper()
    chart, err := FromFS(os.DirFS("testdata/base-chart"))
    require.NoError(t, err)
    for _, mutate := range mutate {
        mutate(chart)
    }
    return chart
}
```

The exact helper name may vary, but it must:

- Load through the public `FromFS` function.
- Load a new chart on every call.
- Apply optional mutations only after a successful load.
- Never cache or share a mutable `*Chart` between subtests.

Delete obsolete in-memory chart builders and serializers after migration,
including `chartForTest`, `yamlObject`, `MarshalJSON`, `templates`, and
`configMapTemplates`, unless a concrete remaining test requires one. Prefer a
small direct `common.File` mutation for an intentionally malformed or special
template over retaining a fixture DSL.

## Happy-Path Render Test

Use one `ToPlainManifests` test row with these direct inputs:

- `chart`: `baseChart(t)`.
- `releaseName`: a non-empty value such as `release`.
- `namespace`: a non-empty value such as `namespace`.
- `options`: `WithValues`, `WithKubeVersion`, and `WithAPIVersions`.

`WithValues` should set:

```go
map[string]any{
    "parent": map[string]any{
        "overridden": "user-parent-override",
        "requiredValue": "user-required-value",
        "replicas": 3,
        "servicePort": 9090,
    },
    "child": map[string]any{
        "userOverride": "user-override",
    },
    "optional": map[string]any{
        "enabled": false,
    },
    "global": map[string]any{
        "shared": "user-global-override",
    },
}
```

Supply a Kubernetes version and API discovery data that let the parent
template assert both a group version and a concrete kind, for example `v1` and
`apps/v1/Deployment`.

The assertion function should verify:

- Rendering succeeds and returns only `*unstructured.Unstructured` objects.
- The parent ConfigMap contains the parent default and caller override.
- The child ConfigMap contains the child default, parent override, and caller
  override.
- The optional child's object is absent because caller values disabled that
  dependency.
- The library helper is consumed successfully and the library dependency does
  not produce a standalone object.
- Parent and child templates see the caller-overridden global value.
- Release name, namespace, fixed upgrade state, and revision are correct.
- Chart metadata, template metadata, `.Files.Get`, `include`, `template`, and
  `required` produce their expected strings and prove their cross-file or value
  relationships.
- Caller-supplied capabilities are visible.
- The CRD is included.
- ConfigMap, Service, and Deployment objects follow Helm install order.
- Templated integer values retain their intended YAML type after rendering and
  decoding.
- `NOTES.txt` successfully evaluates its value and helper references but does
  not produce an object.
- Allowlisted Helm metadata and ordinary metadata are preserved.
- No unexpected extra objects are returned.

Use object kind and name to locate results rather than relying on slice indexes
unless install ordering is itself the behavior being asserted.

This one row should replace separate successful rows for values coalescing,
schema success, supplied capabilities, release and chart context, chart files,
template metadata, named template consumption, arbitrary object decoding,
install ordering, NOTES rendering and exclusion, enabled and disabled
dependency behavior, CRD inclusion, and allowed Helm metadata.

## Error-Path Render Tests

Every non-input renderer error row should call `baseChart(t, mutation)` and
change only the behavior needed for that row. Keep the table case shape aligned
with the public function:

```go
type toPlainManifestsTestCase struct {
    name        string
    chart       *Chart
    releaseName string
    namespace   string
    options     []RenderOption
    assert      func(*testing.T, []client.Object, error)
}
```

Retain focused rows for at least:

- Nil chart.
- Empty release name.
- Empty namespace.
- Invalid Kubernetes version.
- Parent schema violation, using a non-string parent value.
- Child schema violation, using a non-string child value that the parent schema
  does not validate.
- Each policy-significant denied function: `randAlphaNum`, `now`, `uuidv4`, a
  randomness-based crypto function, `getHostByName`, and `lookup`.
- Hook output.
- A disallowed `helm.sh/*` annotation or label.
- Multiple independent validation failures collected into one
  `UnsupportedChart` error.

For template-related mutations, append or replace the smallest possible
`common.File` in the freshly loaded chart. Assertions for unsupported behavior
must use `require.ErrorAs` with `*UnsupportedChart` and check that the error
names the relevant function, field, or metadata key.

If rendering the same chart multiple times is still needed to prove that Helm's
dependency processing does not mutate caller-owned chart state, keep that as a
separate focused test. It should load the base chart once, render with the child
disabled, then render the same chart with the child enabled and verify both
results. This behavior cannot be represented cleanly as a single-call table
row.

## Other Test Files

`TestFromFS` may use `os.DirFS("testdata/base-chart")` as its successful input
and assert the loaded parent metadata, values, schema, CRD, templates, and
application and library dependencies. Keep a separate missing-`Chart.yaml` input
using `fstest.MapFS`.

`TestParseManifest` should remain independent because it tests a string parser,
not chart loading or rendering.

Tests under `internal/hermetic` and `internal/validate` should remain owned by
those packages. Do not duplicate their exhaustive unit cases in
`render_test.go`; renderer tests need only prove integration and public error
typing.

## Implementation Order

1. Create `testdata/base-chart` with parent, application dependencies, a library
   dependency, values, schemas, templates, and CRD.
2. Add the fresh `baseChart` loader helper.
3. Implement the single broad happy-path render row and make it pass.
4. Convert renderer error rows to fresh base-chart mutations.
5. Preserve the focused repeated-render mutation test if still needed.
6. Update `TestFromFS` to use the fixture for its successful case.
7. Delete obsolete in-memory chart and YAML-building helpers.
8. Run Tobari and remove any success tests made redundant by the base-chart row.
9. Run the complete verification commands below.

## Verification

Run:

```sh
GOFLAGS="$(tobari flags)" go test ./bundle/helm/...
make test
make lint
```

Inspect all generated `tobari/tobari.json` or `tobari/tobari.toon` reports. The
single happy-path row should own most successful-path coverage. Error rows
should remain only when they assert a distinct contract or execute a distinct
error branch. Remove generated Tobari report directories after analysis so they
do not remain in the working copy.

## Acceptance Criteria

- `bundle/helm/v1/testdata/base-chart` contains valid parent, application
  dependency, and library charts, with `values.yaml` and `values.schema.json` as
  applicable.
- One successful render verifies parent defaults, parent user overrides, child
  defaults, parent-to-child overrides, child user overrides, capabilities,
  global values, fixed upgrade state, chart and template context, chart file
  access, `include`, `template`, `required`, enabled and disabled dependency
  behavior, library helper consumption, and CRD inclusion.
- The successful render includes ConfigMap, Service, and Deployment objects,
  verifies install ordering, and proves NOTES are templated successfully but
  excluded from returned objects.
- The successful render templates string and integer values and verifies
  decoded integers retain their intended YAML type.
- Parent and child schema failure paths are independently tested.
- All renderer error cases begin with a fresh base chart except nil-input and
  parser-specific tests.
- No mutable chart is shared across independent table rows.
- Obsolete in-memory chart/YAML fixture helpers are removed.
- Tobari shows no semantically redundant renderer tests.
- `GOFLAGS="$(tobari flags)" go test ./bundle/helm/...`, `make test`, and
  `make lint` pass.
