# Verification

## Implementation Correctness

- [ ] `ToPlainManifests` signature is `(b Bundle, opts ...RenderOption)` with no
      positional install namespace.
- [ ] `WithSelfManagedInstallNamespace` is exported from `bundle/registry/v1`
      and wired to the `render` package option.
- [ ] Without the self-managed option, exactly one `Namespace` object is emitted
      and its name follows the precedence chain (template name > suggested
      annotation > `<PackageName>-system`).
- [ ] With the self-managed option, zero `Namespace` objects are emitted and the
      given name is used as the install namespace for all other resources.
- [ ] Every path produces a full `Namespace` object: the template path emits the
      unmarshalled template object (labels/annotations included); the annotation
      and fallback paths emit a Namespace with only the name set.
- [ ] `validateNamespaceName` rejects empty, >63-char, and non-DNS1123 names;
      malformed template JSON yields a render error. Verified for every source.
- [ ] The Namespace object is emitted first (sorts before namespaced resources)
      via `Render` prepending it, not via a generator. There is no
      `BundleNamespaceGenerator` and no `Options.Namespace` field.

## Project Conventions

- [ ] Commits are conventional and imperative, one logical change each
      (specs/conventions.md).
- [ ] Public API change is covered by tests (specs/mission.md: "All public API
      changes require tests").
- [ ] Design keeps types inert with standalone functions; no cluster
      dependencies introduced (specs/mission.md design principles).
- [ ] Reuses `corev1.Namespace` and existing apimachinery validation rather than
      new legacy-dependency usage (specs/tech-stack.md).
- [ ] Implementation detail (resolution, validation) stays in `internal/`
      packages; only the option and entrypoint are public.
- [ ] No emdashes in code, comments, or docs.

## Commands

- [ ] `make lint` passes.
- [ ] `make test` passes.
- [ ] `make ci` passes.
