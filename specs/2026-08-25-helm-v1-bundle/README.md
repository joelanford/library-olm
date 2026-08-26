---
status: in-progress
---
# Helm+v1 Bundle Library

## Summary

A new `bundle/helm/v1` library that renders a Helm chart into `[]client.Object`
in a fully hermetic, client-side way. It is the "Helm" half of mission goal #1
(bundle rendering for registry+v1, Helm, etc.), complementing the existing
`image/bundle/handler_helm.go` chart *unpacking* with chart *rendering*.

The library reimplements `helm template -f <values> <chart>` on top of the
`helm.sh/helm/v4` SDK, using the SDK for as much as possible and deviating from
stock Helm behavior only where our requirements demand it. There are exactly
three deliberate deviations, each enforced and tested:

1. **Hermetic function allowlist** — the template engine may only call functions
   on an explicit allowlist; every other function is overridden to error.
2. **Helm metadata allowlist (validation gate)** — rendered output may only carry
   allowlisted `helm.sh/*` labels and annotations; any other Helm metadata fails
   the render.
3. **No cluster interaction, ever** — no kubeconfig, discovery, live client, or
   `lookup`. Cluster-derived inputs are caller-supplied; Helm's fake printing
   client is used only to satisfy the action API.

## Design

### Guiding principle: use the Helm SDK

Build on the `helm.sh/helm/v4` SDK. Chart loading uses `pkg/chart/v2/loader`, and
rendering uses `action.Install` with server dry-run and a fake printing client.
We do **not** reimplement Helm's render loop, values handling, schema validation,
or Capabilities type. Every deviation below is minimal and explicitly
acknowledged.

Note the v4 package reorg (vs v3): `Capabilities`/`KubeVersion`/`VersionSet` are
in `pkg/chart/common`; release types and manifest splitting are in
`pkg/release/v1` and `pkg/release/v1/util`.

### Public API

Mirrors the `bundle/registry/v1` two-step pattern (`FromFS` → `ToPlainManifests`)
so callers pass an `fs.FS` rather than a pre-loaded chart. Sketch (final names
pinned in implementation):

```go
package v1 // import "github.com/joelanford/library-olm/bundle/helm/v1"

// Chart is a parsed Helm chart (alias for helm's concrete v2 chart type),
// mirroring registry/v1's `type Bundle = ...` alias.
type Chart = chart.Chart // helm.sh/helm/v4/pkg/chart/v2

// FromFS loads a Helm chart from the given filesystem. It walks chartFS into
// helm BufferedFiles (chart-root-relative names) and calls the SDK loader
// (pkg/chart/v2/loader.LoadFiles). No tar/gzip round-trip.
func FromFS(chartFS fs.FS) (*Chart, error)

// ToPlainManifests renders a parsed chart into plain Kubernetes objects,
// equivalent to `helm template <releaseName> -n <namespace>`, with
// hermetic-function and Helm-metadata allowlist enforcement. releaseName and
// namespace are required positional parameters (they populate .Release.Name and
// .Release.Namespace) rather than options, since every render needs them.
func ToPlainManifests(chrt *Chart, releaseName, namespace string, opts ...RenderOption) ([]client.Object, error)

// RenderOptions (functional):
func WithValues(values map[string]any) RenderOption      // merged like `-f`
func WithKubeVersion(kv *version.Info) RenderOption
func WithAPIVersions(groups []*metav1.APIGroup, resources []*metav1.APIResourceList) RenderOption
```

There is intentionally **no** `WithIsUpgrade` / install-vs-upgrade option;
rendering is always pinned to the upgrade steady state. See "Determinism: render
the upgrade steady state" below.

`ToPlainManifests` returns `[]client.Object` as `*unstructured.Unstructured`
values (the chart may contain arbitrary kinds), matching how
`bundle/registry/v1.ToPlainManifests` returns `[]client.Object`. CRDs from
`crds/` are always included (fixed `--include-crds=true`). Order follows helm's
`InstallOrder`.

### Render pipeline (mirrors `helm template`)

0. `FromFS`: `fs.WalkDir(chartFS)` → `[]*archive.BufferedFile{Name: relpath,
   Data: bytes}` (regular files only) → `v2loader.LoadFiles` → `*Chart`. No
   tar/gzip (helm's `LoadArchive` would require gzip + a synthetic top-level
   directory prefix that `tar.Writer.AddFS` cannot add; `LoadFiles` is the same
   SDK code path after those steps, so we call it directly).
1. `ToPlainManifests` receives the parsed `*Chart`.
2. Build empty `common.Capabilities`, applying only caller-supplied Kubernetes
   version and API discovery responses. No cluster discovery occurs.
3. Configure `action.Install` with the supplied release name and namespace,
   fixed upgrade state, CRD inclusion, server dry-run, `hermetic.Overrides()`,
   and `fake.PrintingKubeClient{Out: io.Discard}`.
4. Copy the chart, then run the action. Chart copying prevents Helm dependency
   processing from mutating the caller's chart.
5. Validate that the release has no unexpected fields. Hooks and other unsupported
   release features fail this validation.
6. Split `release.Manifest` using `releaseutil.SplitManifests`, preserve its order
   with `releaseutil.BySplitManifestsOrder`, and decode every document into
   `*unstructured.Unstructured` via `sigs.k8s.io/yaml` and apimachinery.
7. **Helm metadata gate:** collect each non-allowlisted `helm.sh/*` label and
   annotation, then return one `UnsupportedChart` wrapping `errors.Join` of the
   violations.
8. Return the objects as `[]client.Object`.

### Deviation 1 — hermetic function allowlist

Helm's `pkg/engine` builds its FuncMap from `sprig.TxtFuncMap()` plus helm's own
additions, and deletes `env`/`expandenv`. The base `funcMap()` is unexported; the
only hook is `Engine.CustomTemplateFuncs`, merged **last** via `maps.Copy`, which
can override (but not remove) entries. We enforce an allowlist over the **sprig**
functions and explicitly neuter `lookup`:

- **Scope: sprig functions.** Maintain an explicit allowlist of hermetic sprig
  function names. Compute overrides = every key in the *live* `sprig.TxtFuncMap()`
  **not** on the allowlist, each mapped to a stub returning an error like
  `template function "randAlphaNum" is not permitted in hermetic rendering`. This
  is safe-by-default: any function a future pinned sprig version adds is stubbed
  automatically until explicitly allowlisted. (`getHostByName` is a sprig key and
  is handled here — kept off the allowlist so it errors.)
- **Trust helm's own additions.** Helm's non-sprig additions
  (`include`, `tpl`, `required`, `fail`, and the `toYaml`/`fromYaml`/`toJson`/
  `toToml`/… encoders) are hermetic — no randomness, clock, env, or network — so
  we leave them in place and do not attempt to enumerate or guard them. We accept
  the small risk that a future helm version could add a non-hermetic function;
  this keeps the implementation simple and avoids reconstructing an unexported,
  moving funcmap surface. The one helm addition with external reach is `lookup`.
- **Explicitly deny `lookup`.** It is helm-added (not a sprig key). We override
  it via `CustomTemplateFuncs` with an error stub, rather than leaving Helm's
  default placeholder that silently returns an empty map (`funcs.go:73`). This
  ensures a chart cannot make a cluster lookup even though rendering uses the
  action API with a fake client (deviation 3).
- Denied sprig categories include: randomness (`randAlphaNum`, `uuidv4`,
  `randInt`, …), wall-clock (`now`, `date`, `dateInModTime`, `htmlDate`, …),
  randomness-based crypto (`genCA`, `genSignedCert`, `genSelfSignedCert`,
  `derivePassword`, `bcrypt`, `htpasswd`, `genPrivateKey`, …), and host/DNS
  (`getHostByName`). (`env`/`expandenv` are already removed by helm.)
- A guard test asserts allowlist ∪ denied == live `sprig.TxtFuncMap()` keys, so
  sprig drift is caught in CI.

### Deviation 2 — Helm metadata allowlist (validation gate)

A validator, not a transform: it never strips or mutates labels or annotations.
Rendered objects may only carry allowlisted `helm.sh/*` labels and annotations.
All violations are collected and reported in one `UnsupportedChart` error. Other
metadata is not policed by this gate.

### Deviation 3 — no cluster interaction

The library never reads a kubeconfig, performs discovery, or constructs a real
cluster client. `action.Install` receives a fake printing KubeClient and writes
to `io.Discard`, so server dry-run makes no network request. `lookup` is denied
by the hermetic funcmap (deviation 1), so a chart calling it errors rather than
silently receiving Helm's empty-map placeholder. Callers who want
cluster-accurate Capabilities discover the kube version and API versions
themselves and pass them via `WithKubeVersion` and `WithAPIVersions`.

### Hooks

Hook-annotated resources are always rejected by release validation. Helm places
hooks in the release's `Hooks` field, which is not part of the renderer's
supported release shape. Charts intended for this library must not use Helm
hooks.

### Determinism: render the upgrade steady state

The rendered output must be a pure function of `(chart, values, capabilities)`,
so we do not expose an install-vs-upgrade toggle: rendering is pinned to a single
fixed release state. We pin it to the `helm template --is-upgrade` state
(`IsInstall=false`, `IsUpgrade=true`, `Revision=1`), i.e. we always render the
**upgrade** manifest.

Rationale:

- This library produces a desired-state manifest set that an OLM-style reconciler
  applies continuously. Over a release's lifetime the install→upgrade ratio
  approaches zero — almost every reconcile represents an upgrade — so the set the
  reconciler should maintain is the upgrade set.
- Pinning to upgrade avoids the install-then-immediate-upgrade thrash entirely:
  there is no install→upgrade transition because we start (and stay) in the
  upgrade state.
- It is also the *correct* steady state, not merely the common one. Under real
  Helm, `helm upgrade` prunes any object present in the prior manifest but absent
  from the new one. So any object that must persist long-term necessarily appears
  in the upgrade render; if it did not, Helm itself would delete it on the first
  upgrade. Install-only objects (gated on `.Release.IsInstall`) that are absent
  from the upgrade render are exactly the transient ones Helm would have pruned,
  so we lose nothing meant to survive.
- By contrast, pinning to install (`IsInstall=true`) would render transient
  install-only objects that a declarative reconciler would then keep alive
  forever, diverging from Helm's own semantics.

Charts that branch on `.Release.IsInstall`/`.Release.IsUpgrade` still render
deterministically (always the upgrade path); their install-only paths are
intentionally not emitted. This is the same determinism guarantee as the hermetic
funcmap, applied at the release-metadata layer.

### `helm template` flag mapping

| `helm template` behavior | Decision |
|---|---|
| cluster interaction | None, ever. Caller supplies cluster-derived values. |
| dry-run / server-side (SDK settings) | Server dry-run with a fake printing client. No network access. |
| `--include-crds` | Fixed `true`. |
| `--kube-version` | `WithKubeVersion`, caller-supplied. |
| `--api-versions` | `WithAPIVersions`, caller-supplied. |
| `--is-upgrade` | No toggle. Always rendered as upgrade (`IsInstall=false`, `IsUpgrade=true`, `Revision=1`). See "Determinism: render the upgrade steady state". |
| release name / namespace | Required positional params of `ToPlainManifests` (populate `.Release.Name` / `.Release.Namespace`). |
| `-f`/`--values` | `WithValues(map[string]any)`. Map only; no `--set` string parsing. |
| `-s`/`--show-only` | Omitted; callers filter the returned slice. |
