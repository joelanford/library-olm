# Verification

## Implementation Correctness

- [ ] Generators receive a copied bundle struct and options as separate,
      read-only inputs. `GeneratorContext` owns only the resolved install
      namespace and accumulated objects.
- [ ] Pipeline execution follows declared order, shares render-produced state
      with later stages without mutating bundle, nested reference values, or
      options inputs, and
      short-circuits on an error without returning partial output.
- [ ] Bundle validation and render-option defaults occur before pipeline stages.
- [ ] `NamespaceGenerator` is first, resolves and validates the install and
      target namespaces, stores the install namespace in the context, and emits
      a derived Namespace first only when it is not self-managed.
- [ ] Ordinary Registry+v1 resource stages preserve their current relative
       output order and rendered content.
- [ ] Certificate-provider mutations remain in the resource generators that
      construct their affected resources. `CertProviderResourceGenerator` runs
      after ordinary stages and appends provider-owned resources last.
- [ ] Nil providers are no-ops; provider errors produce no partial output. No
      certificate-provider centralization is introduced.
- [ ] `ToPlainManifests` and exported render options are unchanged and retain
      their tested behavior.
- [ ] Existing tests are adapted for the new internal function and object
      shapes; no behavior-focused test cases are added for this refactor.

## Project Conventions

- [ ] Commits are conventional, imperative, and each contains one logical
      change (specs/conventions.md).
- [ ] Existing tests cover the refactored internal shapes and preserve overall
      project coverage (specs/conventions.md).
- [ ] The refactor preserves inert data types and uses standalone functions;
      internal pipeline details remain in `internal/` (specs/mission.md).
- [ ] No cluster dependencies or unnecessary legacy dependencies are introduced
      (specs/mission.md and specs/tech-stack.md).
- [ ] Go 1.25.7 project conventions and existing Go testing dependencies are
      used (specs/tech-stack.md).
- [ ] No `//nolint` suppression comments are added.

## Commands

- [ ] `make lint` passes.
- [ ] `make test` passes.
- [ ] `make ci` passes.
