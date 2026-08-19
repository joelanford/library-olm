# Requirements

## Functional

- `ToPlainManifests` no longer takes a positional `installNamespace` argument.
- A new render option `WithSelfManagedInstallNamespace(name string)` declares
  that the caller manages the namespace of the given name.
- When `WithSelfManagedInstallNamespace` is supplied:
  - The install namespace used for all generated resources is `name`.
  - No `Namespace` object is present in the rendered output.
- When `WithSelfManagedInstallNamespace` is NOT supplied, a full
  `corev1.Namespace` object is resolved from the CSV annotations using this
  precedence:
  1. `operatorframework.io/suggested-namespace-template` - unmarshal the
     JSON/YAML value directly into a `corev1.Namespace`; the unmarshalled object
     is used as-is (its labels/annotations come along naturally). Its
     `.metadata.name` is the resolved name. When present, the template is
     authoritative: there is NO fallback to options 2 or 3 for the name. An empty
     template name is a validation error.
  2. `operatorframework.io/suggested-namespace` - construct a `corev1.Namespace`
     with only `.metadata.name` set to the annotation value, used only if the
     template annotation is absent.
  3. `<PackageName>-system` - construct a `corev1.Namespace` with only
     `.metadata.name` set, used only if both annotations are absent.
- When NOT self-managed, the resolved `Namespace` object is included in the
  output and sorts before namespaced resources.
- All three paths produce a full `Namespace` object; they differ only in the
  object's source (unmarshalled blob vs. constructed with a name). Metadata is
  not plumbed separately.
- The resolved name is validated by `validateNamespaceName`: non-empty,
  length <= 63, valid DNS1123 label. `Render` returns an error on failure.
- Name validation applies to all sources, including a self-managed name and the
  `<PackageName>-system` fallback.

## Error handling

- A malformed `suggested-namespace-template` JSON value causes `Render` to
  return a descriptive error.
- An invalid resolved name (any source) causes `Render` to return a descriptive
  error identifying the offending name.

## Acceptance Criteria

- [ ] `ToPlainManifests(b, opts...)` compiles with no positional namespace arg;
      all in-repo callers and tests updated.
- [ ] Rendering without `WithSelfManagedInstallNamespace` emits exactly one
      `Namespace` object whose name matches the precedence chain.
- [ ] Rendering with `WithSelfManagedInstallNamespace("foo")` emits no
      `Namespace` object and uses `foo` as the install namespace everywhere.
- [ ] Template path: the emitted Namespace equals the unmarshalled template
      object (labels/annotations included); annotation and fallback paths emit a
      Namespace with only the name set.
- [ ] `suggested-namespace` is used only when the template annotation is absent.
- [ ] When the template is present, resolution never falls back to
      `suggested-namespace` or `<PackageName>-system`; an empty template name is
      a validation error.
- [ ] `<PackageName>-system` is used only when both annotations are absent. A
      present-but-empty `suggested-namespace` yields an empty name and is a
      validation error, not a fallthrough.
- [ ] Invalid names (empty, >63 chars, non-DNS1123) from every source produce a
      render error; malformed template JSON produces a render error.
- [ ] All new public API surface has tests (per mission: public API changes
      require tests).
- [ ] `make ci` passes.
