---
status: done
---
# Managed Namespaces

## Summary

Registry+v1 rendering currently requires the caller to pass a concrete
`installNamespace` positionally to `ToPlainManifests`. This work item makes the
install namespace caller-optional: when the caller does not declare that it
manages the namespace itself, the library derives the install namespace name
from the bundle's CSV annotations (with a package-based fallback) and emits a
generated `Namespace` object as part of the rendered manifest set.

This lets consumers render a self-contained, directly-applyable manifest set
without hardcoding namespace knowledge, while still allowing callers that own
namespace lifecycle to opt out of Namespace generation.

## Design

### API change (breaking)

The positional `installNamespace string` argument is removed from the public
render entrypoint and the internal renderer, replaced by an option:

```go
// before
func ToPlainManifests(b Bundle, installNamespace string, opts ...RenderOption) ([]client.Object, error)

// after
func ToPlainManifests(b Bundle, opts ...RenderOption) ([]client.Object, error)

// new option
var WithSelfManagedInstallNamespace = render.WithSelfManagedInstallNamespace
```

`WithSelfManagedInstallNamespace(name string)` declares that the caller manages
the namespace with the given name. When supplied:
- The install namespace is exactly `name`.
- No `Namespace` object is emitted (the caller owns its lifecycle).

When NOT supplied:
- The library derives the `Namespace` object via the precedence chain below.
- That generated `Namespace` object is included in the output.

In all cases the resolved namespace name is run through `validateNamespaceName`
and a render error is returned on failure.

### Namespace resolution precedence

Resolution produces a full `corev1.Namespace` object (not just a name). It is
resolved from the CSV's `.metadata.annotations`, in order:

1. `operatorframework.io/suggested-namespace-template` - if present, unmarshal
   its JSON/YAML value directly into a `corev1.Namespace`. The unmarshalled
   object is the Namespace, so any labels/annotations the template carries (e.g.
   Pod Security Admission labels) are naturally present. Its `.metadata.name` is
   the resolved name. When the template is present it is authoritative: we do NOT
   fall back to options 2 or 3 to source a name. If its name is empty, that is a
   validation error (see below), not a fallthrough.
2. `operatorframework.io/suggested-namespace` - construct a `corev1.Namespace`
   with `.metadata.name` set to the annotation's string value. Used only if the
   template annotation is absent.
3. `<PackageName>-system` - construct a `corev1.Namespace` with `.metadata.name`
   set to `<rv1.PackageName>-system`. Used only if both annotations are absent.

The difference between the paths is only the source of the object: option 1
unmarshals a full object from a blob, options 2 and 3 construct one with just
the name set. Metadata is not plumbed separately from the Namespace object.

### Name validation

`validateNamespaceName(name string) error` enforces:
- non-empty
- length <= 63
- valid DNS1123 label (`k8s.io/apimachinery/pkg/util/validation.IsDNS1123Label`)

Applied to the resolved name regardless of source (self-managed, template,
annotation, or fallback).

### Rendering flow

Namespace resolution happens in `BundleRenderer.Render` **before** the resource
generators run, so that `Options.InstallNamespace` (a plain `string`) is
populated for every existing generator exactly as today. Resolution is a single
internal function:

```go
func resolveInstallNamespace(rv1 *bundle.RegistryV1, selfManaged *string) (namespace corev1.Namespace, emit bool, err error)
```

- `namespace` always carries the resolved install namespace (its `.Name` is the
  install namespace name).
- `emit` is false when the caller self-manages the namespace, true when the
  library derived it.

The install namespace name is not the generators' to choose - it is a universal
input, resolved centrally like `defaultTargetNamespacesForBundle` already
resolves `TargetNamespaces`. `Render` sets `Options.InstallNamespace = ns.Name`
and, when `emit` is true, **prepends the resolved Namespace object to the output**
so it sorts before namespaced resources:

```go
var objs []client.Object
if emit {
    objs = append(objs, &ns)
}
generated, err := ResourceGenerators(r.ResourceGenerators).GenerateResources(&rv1, *genOpts)
// ...
objs = append(objs, generated...)
```

The only new field on `render.Options` is `SelfManagedInstallNamespace *string`
(the option input; nil means "derive"). There is **no** `BundleNamespaceGenerator`
and no `Options.Namespace` field: the Namespace object is install scaffolding, not
bundle content, so `Render` emits it directly rather than routing it through a
pass-through generator. This keeps ordering structural (not dependent on generator
list position) and keeps generators as pure functions of the resolved name.

> Note: this design was chosen over an earlier "namespace generator" approach.
> A generator that only forwards a Render-made decision is a fake generator, and
> it reintroduces a list-position ordering dependency. A future rework may recast
> all generators as a transform pipeline over a shared render context, at which
> point install namespace resolution becomes the first transform - but that is a
> separate work item.

### Non-goals

- No changes to target-namespace / install-mode logic; install namespace
  resolution is independent of `WithTargetNamespaces`.
- No new public Namespace type; reuse `corev1.Namespace`.
