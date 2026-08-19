# Implementation Plan

Follow TDD: add/adjust tests alongside each task group. Use `jj new -m "..."`
to seal each task group before starting the next.

## 1. Namespace resolution + validation (internal/render)

- Add `validateNamespaceName(name string) error` in the `render` package:
  non-empty, `len <= 63`, and `validation.IsDNS1123Label` from
  `k8s.io/apimachinery/pkg/util/validation`.
- Add annotation constants:
  - `annotationSuggestedNamespaceTemplate = "operatorframework.io/suggested-namespace-template"`
  - `annotationSuggestedNamespace = "operatorframework.io/suggested-namespace"`
- Add `resolveInstallNamespace(rv1 *bundle.RegistryV1, selfManaged *string)` that
  returns `(namespace corev1.Namespace, emit bool, err error)`:
  - If `selfManaged != nil`: validate the name and return a Namespace whose name
    is the given value with `emit = false` (the caller owns the namespace;
    nothing is emitted). Its `.Name` still flows to `Options.InstallNamespace`.
  - Else apply the precedence chain, producing a full `corev1.Namespace` with
    `emit = true`: template unmarshalled from the blob (authoritative when
    present - no fallback for the name), or constructed with just the name set for
    the annotation/fallback paths. Validate `ns.Name` via `validateNamespaceName`
    (an empty template name fails here rather than falling through).
- Unit tests for each precedence branch, template unmarshal (valid/invalid),
  template-present-is-authoritative (empty template name errors, no fallback),
  suggested-namespace present-but-empty errors (no fallback), fallback used only
  when both annotations absent, self-managed (`emit == false`), and validation
  failures.

## 2. Options + option constructor + Render (internal/render)

- Extend `render.Options` with a single new field `SelfManagedInstallNamespace
  *string` (the option input; nil means "derive"). `Options.InstallNamespace`
  stays a plain `string`. No `Options.Namespace` field is added.
- Add `WithSelfManagedInstallNamespace(name string) Option`.
- Update `BundleRenderer.Render` to drop the positional `installNamespace`,
  resolve the namespace (task 1) after applying options, set
  `Options.InstallNamespace = ns.Name`, and - when `emit` is true - prepend the
  resolved Namespace object to the output before appending the generator output.
  There is no namespace generator; ordering-first is guaranteed structurally.

## 3. Public API (bundle/registry/v1)

- Change `ToPlainManifests(b Bundle, opts ...RenderOption)` (drop positional).
- Export `WithSelfManagedInstallNamespace = render.WithSelfManagedInstallNamespace`.
- Update package doc comments and `example_test.go`.

## 4. Update callers and tests

- Update all in-repo callers of `ToPlainManifests` / `Renderer.Render`
  (including `registryv1_test.go`, `regression_test.go`) to the new signature,
  using `WithSelfManagedInstallNamespace` where a fixed namespace was intended.
- Add end-to-end tests through `ToPlainManifests` for the derived-namespace and
  self-managed paths.

## 5. Finalize

- `make lint-fix`, `make test`, `make ci`.
- Confirm no lingering references to the removed positional argument.
