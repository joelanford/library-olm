# Requirements

## Functional

- Replace the internal resource-generator contract with a copied Registry+v1
  bundle struct, read-only render-options input, and a shared
  `GeneratorContext` containing render-produced state: the resolved install
  namespace and accumulated `client.Object` values.
- Generators must not mutate their bundle or options inputs, including nested
  reference values.
- Execute resource generators in declared order, append generated objects to the
  context, and stop at the first generator error.
- Preserve `ToPlainManifests` and all exported render options without signature
  or behavioral changes.
- Retain bundle validation and default option initialization before the pipeline
  runs.
- Add `NamespaceGenerator` as the first Registry+v1 stage. It must resolve the
  install namespace using the existing self-managed and annotation precedence,
  validate the namespace and target namespaces, set
  `GeneratorContext.InstallNamespace`, and append a Namespace only when it is
  derived.
- Convert the existing resource generators to append resources through the
  context in their present relative order, retaining their existing local
  certificate-provider behavior.
- Retain `CertProviderResourceGenerator` as the final stage that appends
  provider-owned resources for webhook-serving deployments.
- A nil certificate provider must retain its current no-op behavior. Generator
  or certificate-provider errors must cause rendering to return no object slice
  and the underlying error, consistent with current rendering.
- Do not centralize certificate-provider mutations in this refactor.

## Acceptance Criteria

- [ ] The public `ToPlainManifests` API and exported options compile unchanged;
      all existing external-facing behavior remains covered by tests.
- [ ] Existing unit tests are updated for generators that receive a copied
      bundle struct and separate options input, share render-produced state
      through one context without mutating those inputs, run in declared order,
      and stop at the first error without returning partial output.
- [ ] A derived Namespace is first in the output, while a self-managed namespace
      emits none and is available to every following stage as the install
      namespace.
- [ ] Existing namespace precedence, name validation, target-namespace
      validation, and default target namespace tests continue to pass.
- [ ] The normal Registry+v1 stages preserve their current relative output
      order.
- [ ] Existing certificate-provider tests continue to cover local mutations,
      no-op behavior, errors, and provider-owned objects appended last.
- [ ] The refactor adds no behavior-focused test cases. Existing tests are
      adapted only as required by the internal generator and context shapes.
- [ ] `make ci` passes.
