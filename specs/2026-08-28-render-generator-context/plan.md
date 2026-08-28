# Implementation Plan

Update existing tests with each task group to reflect the new internal function
and object shapes. Do not add behavior-focused test cases for this
behavior-preserving refactor. Use `jj new -m "..."` to seal each completed
logical task group before starting the next.

## 1. Introduce the internal context pipeline

- Define `GeneratorContext` in `bundle/registry/v1/internal/render` with the
  resolved install namespace and accumulated object slice. Pass a copied bundle
  struct and options separately as read-only generator inputs.
- Change `ResourceGenerator`, `ResourceGenerators`, and `BundleRenderer` to
  construct and run the context sequentially, preserving bundle validation,
  option defaults, error propagation, and the no-partial-output contract.
- Update existing focused `render` tests for order, shared render-state
  mutation, a copied bundle input, immutable options input, and short-circuiting
  after a generator error.

## 2. Make namespace handling the first stage

- Convert existing namespace resolution into `NamespaceGenerator`.
- Resolve and validate the install namespace and target namespaces in this
  stage, store the resolved name in the context, and append a derived
  Namespace object when appropriate.
- Put this generator first in the Registry+v1 stage list and adapt existing tests to
  assert Namespace-first ordering, self-managed behavior, and availability of
  the resolved namespace to later generators.

## 3. Convert resource generators

- Convert the service account, permission, cluster permission, CRD,
  additional-resource, deployment, validating webhook, mutating webhook, and
  service generators to append their resources to the context.
- Retain certificate-provider operations in the deployment, CRD, webhook, and
  Service generators.
- Adapt generator and Registry+v1 renderer tests for the new internal
  contract.

## 4. Retain final certificate-provider resources

- Convert `CertProviderResourceGenerator` into the final context generator,
  retaining its existing responsibility to append provider-owned resources.
- Retain the existing certificate provisioner naming, provider interface, and
  generator-local mutations. Defer centralizing those mutations to separate
  work.
- Adapt existing certificate-provider tests for the new internal contract.

## 5. Finalize

- Run `make lint-fix`, `make test`, and `make ci`.
- Confirm the public render API and generator-local certificate-provider
  behavior are unchanged.
