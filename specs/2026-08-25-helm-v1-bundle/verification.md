# Verification

## Implementation Correctness

- [ ] `FromFS` loads a multi-file chart (subchart + crds) from an `fs.FS` via
      WalkDir → `LoadFiles`; errors on missing `Chart.yaml`.
- [ ] `ToPlainManifests` output matches `helm template` golden output for a
      multi-resource chart given identical values, release name, namespace, and
      Capabilities.
- [ ] `WithValues` values coalesce over chart defaults exactly as Helm does.
- [ ] `values.schema.json` validation: invalid values fail `ToPlainManifests`;
      valid values succeed.
- [ ] No install-vs-upgrade option exists; render uses the
      `helm template --is-upgrade` state (`IsInstall=false`, `IsUpgrade=true`,
      `Revision=1`). A chart branching on `.Release.IsUpgrade` renders its upgrade
      path deterministically on every call; install-only paths are not emitted.
- [ ] CRDs under `crds/` appear in the output (include-crds fixed true).
- [ ] Subcharts under `charts/` render with no network access.
- [ ] Denied template functions (`randAlphaNum`, `now`, `uuidv4`, a randomness
      crypto function, `getHostByName`, and `lookup`) each cause
      `ToPlainManifests` to fail with an error naming the function. In particular
      `lookup` errors rather than returning helm's silent empty-map placeholder.
- [ ] Allowed functions (`upper`, `default`, `b64enc`, `toYaml`, `include`,
      `required`) render successfully.
- [ ] Guard test asserts allowed ∪ denied == live `sprig.TxtFuncMap()` keys and
      fails on drift (sprig-only; helm's additions are trusted, not guarded).
- [ ] Helm metadata gate runs over decoded `*unstructured.Unstructured` objects
      (using `obj.GetAnnotations()` and `obj.GetLabels()`), after decoding: every
      non-allowlisted `helm.sh/*` annotation or label is reported in one
      `UnsupportedChart` error; allowlisted and non-`helm.sh/` metadata passes;
      no metadata is mutated.
- [ ] Hooks and all other unexpected Helm release fields fail
      `ToPlainManifests` with an `UnsupportedChart` error.
- [ ] Rendering does not read kubeconfig, perform discovery, or make network
      calls. Helm's fake printing KubeClient and the `client.Object` API are
      intentional dependencies.

## Project Conventions

- [ ] Conventional commit messages, imperative, <= 72 char subjects, one logical
      change per commit (`specs/conventions.md`).
- [ ] Public API changes have tests; new code >= 70% coverage; overall coverage
      does not decrease.
- [ ] `make ci` passes (lint, verify, test, build) with the
      `containers_image_openpgp` tag.
- [ ] No `//nolint` added without explicit permission.

## Mission Design Principles (`specs/mission.md`)

- [ ] Pure data types with standalone functions; `FromFS`/`ToPlainManifests` are functions, not methods coupling logic to a bespoke type.
- [ ] No cluster interaction at runtime — no kubeconfig, discovery, live client,
      or network calls.
- [ ] Legacy dependencies (`operator-framework/*`) are not introduced.
- [ ] Implementation details live under `bundle/helm/v1/internal/`; public API
      surface is minimal.

## Tech Stack (`specs/tech-stack.md`)

- [ ] Uses `helm.sh/helm/v4` SDK for loading, values/coalesce/schema,
      Capabilities, and rendering rather than reimplementing them.
- [ ] Reuses existing deps (`santhosh-tekuri/jsonschema/v6` via the SDK; no new
      JSON schema dependency added directly).
- [ ] The three deviations are the only departures from stock Helm behavior and
      are documented in the PR body.
