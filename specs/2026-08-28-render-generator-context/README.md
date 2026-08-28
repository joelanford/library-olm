---
status: in-progress
---
# Render Generator Context

## Summary

Refactor registry+v1 rendering into an ordered, shared-context pipeline without
changing its public API or rendered output. The pipeline makes namespace
resolution an explicit first stage while retaining certificate-provider behavior
in the generators that construct the affected resources.

This replaces the current special-case orchestration in `BundleRenderer.Render`
and independent `(bundle, options) -> objects` generators with a consistent
internal model for future rendering transforms.

## Design

### Internal pipeline

The `render` package gains an internal `GeneratorContext` for render-produced
state only:

```go
type GeneratorContext struct {
	InstallNamespace string
	Objects          []client.Object
}

type ResourceGenerator func(bundle.RegistryV1, Options, *GeneratorContext) error
```

`Bundle` and `Options` are separate, read-only generator inputs. Generators
must not mutate either input or their nested reference values. The bundle is
passed by value so each generator receives a copied top-level struct.
`ResourceGenerators` invokes generators in slice order and stops at the first
error. Each generator appends to `ctx.Objects` rather than returning a separate
slice. `BundleRenderer.Render` continues to validate the bundle and initialize
default options before it creates the context and runs the pipeline. An error
from any stage returns no rendered objects, as today.

This is entirely internal. `ToPlainManifests`, the exported render options, and
their behavior remain unchanged. `BundleRenderer` and generator configuration
are internal implementation details and may adopt the new context shape.

### Ordered stages

The registry+v1 generator list is ordered as follows:

1. `NamespaceGenerator` resolves the install namespace from
   `WithSelfManagedInstallNamespace` or the existing CSV annotation precedence,
   validates the resolved name and target namespaces, stores the name in
   `ctx.InstallNamespace`, and appends the derived Namespace object when
   it is not self-managed.
2. The existing service account, permission, cluster permission, CRD,
   additional-resource, deployment, webhook, and service generators append
   their ordinary resources in their current relative order.
3. `CertProviderResourceGenerator` runs after all normal generators and appends
   provider-owned resources.

The namespace stage is the sole producer of a managed Namespace. Its first
position preserves the existing Namespace-first output ordering and ensures all
following stages receive the resolved install namespace from the context without
mutating render options.

### Certificate providers

Certificate-provider behavior remains local to the deployment, CRD, validating
webhook, mutating webhook, and Service generators. Each generator derives its
certificate provisioner from its deployment name and the resolved install
namespace in the context, preserving existing mutations and error behavior.
`CertProviderResourceGenerator` continues to append provider-owned objects
last. A nil certificate provider remains a no-op.

Centralizing certificate-provider mutations in a final pass is intentionally
out of scope for this refactor.

### Behavior guarantees

For equivalent inputs, successful renders contain the same object types,
content, and order as before this refactor. Existing namespace resolution,
install-mode validation, target-namespace defaults, error behavior, and
certificate-provider semantics are retained. Focused tests exercise pipeline
order and error short-circuiting, plus rendered namespace and certificate
outcomes by updating the existing test suite for the new internal function and
object shapes, rather than adding behavior tests or serialized golden fixtures.
