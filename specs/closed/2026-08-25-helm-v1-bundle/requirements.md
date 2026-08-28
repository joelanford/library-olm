# Requirements

## Functional

- Provide a new package `github.com/joelanford/library-olm/bundle/helm/v1`,
  mirroring `bundle/registry/v1`'s `FromFS` / `ToPlainManifests` pattern.
- `FromFS(chartFS fs.FS) (*Chart, error)`: load a Helm chart from a filesystem by
  walking it into `[]*archive.BufferedFile` (chart-root-relative names, regular
  files only) and calling `pkg/chart/v2/loader.LoadFiles`. No tar/gzip
  round-trip.
- `ToPlainManifests(chrt *Chart, releaseName, namespace string, opts
  ...RenderOption) ([]client.Object, error)`: render a parsed chart into
  `[]client.Object` equivalent to
  `helm template <releaseName> -n <namespace> -f <values> <chart>`. `releaseName`
  and `namespace` are required positional parameters (they populate
   `.Release.Name` / `.Release.Namespace`), not options. Uses `action.Install`
   with a `fake.PrintingKubeClient`, server dry-run, and custom hermetic template
   functions. Validate the returned release contains only expected fields; this
   rejects hooks and any other release features the renderer does not support.
- Support functional render options:
  - `WithValues(map[string]any)` — user values, coalesced over chart defaults
    exactly as Helm does (map input only; no `--set` string parsing).
  - `WithKubeVersion(*version.Info)` — sets Capabilities.KubeVersion from the
    caller-supplied discovery ServerVersion response.
  - `WithAPIVersions([]*metav1.APIGroup, []*metav1.APIResourceList)` — sets
    Capabilities.APIVersions from caller-supplied discovery responses.
- Start with empty Capabilities. The caller supplies all Kubernetes version and
  API-version information, so the output has no implicit cluster assumptions.
- Render is always pinned to the `helm template --is-upgrade` release state
  (`IsInstall=false`, `IsUpgrade=true`, `Revision=1`) — the upgrade steady state.
  There is no install-vs-upgrade option: output must be a pure function of chart +
  values + capabilities. Charts that branch on `.Release.IsInstall`/`.IsUpgrade`
  always render the upgrade path; install-only paths are intentionally not
  emitted (they are the transient objects Helm would prune on first upgrade).
- Always include CRDs from the chart's `crds/` directory in the output.
- Validate merged user values against `values.schema.json` when the chart
  contains one, using the SDK's validator; surface validation errors clearly.
- Return objects as `*unstructured.Unstructured` satisfying `client.Object`,
  ordered by Helm's install order, with `NOTES.txt` excluded.
- Return `*UnsupportedChart` for a denied template function, unexpected release
  field, or disallowed Helm metadata. `UnsupportedChart` unwraps the cause so
  callers can inspect it with `errors.As` or `errors.Is`.

## Hermetic function allowlist (deviation 1)

- Maintain an explicit allowlist of hermetic **sprig** function names.
- Override every key in the live `sprig.TxtFuncMap()` not on the allowlist with a
  stub returning a descriptive error naming the function (safe-by-default: unknown
  future sprig functions are stubbed until explicitly allowlisted).
- Trust helm's own additions (`include`, `tpl`, `required`, `fail`, and the
  `toYaml`/`fromYaml`/`toJson`/`toToml` encoders): they are hermetic and left in
  place. We do not enumerate or guard helm's addition set, accepting the small
  risk of a future non-hermetic helm addition in exchange for simplicity.
- Explicitly override `lookup` (helm-added, not a sprig key) with an error stub,
  so a chart calling `lookup` fails loudly rather than receiving helm's silent
  empty-map placeholder.
- Denied sprig functions must include all randomness, wall-clock/date,
  randomness-based crypto, and host/DNS (`getHostByName`) functions.
- Rendering a chart that calls a denied function (or `lookup`) must fail with that
  function's error, not silently produce output.

## Helm metadata allowlist (deviation 2)

- Maintain allowlists of innocuous `helm.sh/*` annotation and label keys.
- After rendering, fail if any rendered object carries a `helm.sh/*` annotation
  or label not on the relevant allowlist; the error must identify each offending
  object and key. Return all violations as `UnsupportedChart{Err:
  errors.Join(...)}`.
- The gate never strips or mutates labels or annotations. Non-`helm.sh/`
  metadata is not policed.
- Hooks are rejected by release-field validation before manifest decoding.

## No cluster interaction (deviation 3)

- The renderer must not read kubeconfig, create a real client, or perform
  discovery. It uses Helm's fake printing client only to satisfy `action.Install`;
  no network request is made.
- `lookup` is denied by the hermetic funcmap, so it errors rather than reaching a
  cluster or returning Helm's silent empty-map placeholder.

## Acceptance Criteria

- `ToPlainManifests` of a representative chart (deployment + service + configmap) produces
  the same objects as `helm template` for the same values, release name,
  namespace, and Capabilities (verified against golden output).
- A chart that references `values.schema.json` fails `ToPlainManifests` when given values
  that violate the schema, and succeeds with valid values.
- A chart template calling a denied function (e.g. `randAlphaNum`, `now`,
  `uuidv4`, `lookup`) fails `ToPlainManifests` with an `UnsupportedChart` error
  naming that function.
- A chart template calling an allowed function (e.g. `upper`, `toYaml`,
  `include`, `required`, `default`, `b64enc`) renders successfully.
- A chart producing an object with a non-allowlisted `helm.sh/*` annotation fails
  `ToPlainManifests`; the same chart without it succeeds.
- A chart with any `helm.sh/hook` resource fails `ToPlainManifests` with an
  `UnsupportedChart` error.
- CRDs under `crds/` appear in the output.
- Subcharts under `charts/` render without any network access.
- A guard test asserts allowlist ∪ denied == live `sprig.TxtFuncMap()` keys and
  fails on drift (sprig-only; helm's additions are trusted, not guarded).
- Rendering does not read kubeconfig, perform discovery, or make network calls.
- New code has >= 70% statement coverage; overall coverage does not decrease.
