# Implementation Plan

Each numbered group is intended to be an independently reviewable commit. Use the
`containers_image_openpgp` build tag for all builds/tests (via the Makefile).

## 1. Package scaffold and FromFS

- Create `bundle/helm/v1/` with a package doc comment describing the hermetic,
  client-side, `helm template`-equivalent contract.
- Add `type Chart = chart.Chart` alias (`pkg/chart/v2`), mirroring registry/v1's
  `type Bundle = ...`.
- Implement `FromFS(chartFS fs.FS) (*Chart, error)`: `fs.WalkDir` regular files
  into `[]*archive.BufferedFile{Name: relpath, Data: bytes}` and call
  `pkg/chart/v2/loader.LoadFiles`. (No tar/gzip: `LoadArchive` requires gzip and
  strips a synthetic top-level directory that `tar.Writer.AddFS` cannot inject;
  `LoadFiles` is the same SDK path after those steps.)
- Add `testdata/` charts used here and by later groups.
- Unit tests: `FromFS` loads a multi-file chart (with subchart + crds) from a
  `testfs`/`fstest.MapFS`; error on missing `Chart.yaml`.

Commit: `feat: add bundle/helm/v1 FromFS chart loading`

## 2. Hermetic function allowlist

- Add an internal package (e.g. `bundle/helm/v1/internal/hermetic`) exposing:
  - `AllowedSprigFuncs() map[string]struct{}` — the curated allowlist of hermetic
    sprig function names.
  - `Overrides() template.FuncMap` — for every key in the live
    `sprig.TxtFuncMap()` not in the allowlist, a stub returning
    `fmt.Errorf("template function %q is not permitted in hermetic rendering", name)`;
    plus an explicit `"lookup"` entry mapped to the same error stub (helm-added,
    not a sprig key). Helm's other additions are trusted and left untouched.
- Guard test: assert allowlist ∪ denied == live `sprig.TxtFuncMap()` keys; fail
  on drift (sprig-only — helm's additions are trusted, not enumerated/guarded).
- Unit tests: a denied sprig function stub errors; `lookup` errors; allowed names
  are absent from the override map.

Commit: `feat: add hermetic template function allowlist`

## 3. Render pipeline

- Implement `ToPlainManifests(chrt *Chart, releaseName, namespace string, opts
  ...RenderOption) ([]client.Object, error)` (release name and namespace are
  required positional params) and the `RenderOption` functional options
  (`WithValues`, `WithKubeVersion(*version.Info)`, and
  `WithAPIVersions([]*metav1.APIGroup, []*metav1.APIResourceList)`). No
  install-vs-upgrade option (see README "Determinism: render the upgrade steady
  state").
- Build empty Capabilities from caller-supplied Kubernetes discovery data.
- Configure `action.Install` for server dry-run with a fake printing KubeClient,
  fixed upgrade state, CRD inclusion, and `hermetic.Overrides()`.
- Copy the chart before rendering because Helm mutates chart state when processing
  dependencies. Run the install action, validate its release contains only
  expected fields, and reject all unsupported fields, including hooks.
- Parse `release.Manifest` with `releaseutil.SplitManifests`, preserving its
  manifest order with `releaseutil.BySplitManifestsOrder`, then decode documents
  to `[]*unstructured.Unstructured` (`sigs.k8s.io/yaml` + apimachinery).
- Unit tests: golden-output comparison against `helm template` for a
  multi-resource chart; schema-valid vs schema-invalid values; subchart
  rendering; CRD inclusion.

Commit: `feat: add hermetic helm chart rendering`

## 4. Helm metadata gate and typed errors

- Add allowlists of innocuous `helm.sh/*` annotation and label keys.
- Run the gate over the already-decoded `[]*unstructured.Unstructured` so it
  reads structured labels and annotations via `obj.GetLabels()` and
  `obj.GetAnnotations()` rather than parsing YAML. Collect every non-allowlisted
  `helm.sh/*` label or annotation and return one `UnsupportedChart` whose cause
  is `errors.Join` of all violations. Never mutate metadata.
- Return `UnsupportedChart` as well for denied template functions and unexpected
  release fields, retaining each underlying cause through `Unwrap`.
- Unit tests: denied template function, hook, unexpected release field, and
  non-allowlisted Helm metadata return `UnsupportedChart`; multiple metadata
  violations are all reported.

Commit: `feat: add helm.sh annotation validation gate`

## 5. Runtime no-cluster guard and docs

- Add a test or documentation asserting the renderer does not read kubeconfig,
  perform discovery, or make network calls. Helm's fake printing KubeClient and
  `client.Object` API are intentional dependencies.
- Add an `example_test.go` demonstrating `FromFS` + `ToPlainManifests`.
- Ensure `make ci` passes (lint, verify, test, build).

Commit: `test: add no-cluster import guard and helm+v1 example`

## Deviations to acknowledge in the PR body

1. Hermetic allowlist over sprig functions (override every non-allowlisted
   `sprig.TxtFuncMap()` key to error) plus an explicit `lookup` override, via
   `Engine.CustomTemplateFuncs`. Helm's own additions are trusted as hermetic.
   (Helm's base funcMap is unexported and cannot be replaced; override-to-error is
   the minimal SDK-compatible mechanism.)
2. Helm metadata allowlist validation gate on rendered output (`helm.sh/*` labels
   and annotations), collecting all violations in one `UnsupportedChart` error.
3. No cluster interaction: `action.Install` uses a fake printing client; no
   kubeconfig or discovery; `lookup` is denied by the funcmap; Capabilities are
   fully caller-supplied.
