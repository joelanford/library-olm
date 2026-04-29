# Mission

library-olm provides reusable Go building blocks for OLM's core functionality. It is a library — not a CLI, not a controller, not an application.

## Goals

1. **Bundle and catalog definitions and transformations** — parsing, validating, and rendering bundles in various formats (registry+v1, Helm, etc.); defining catalog data models and format conversions.
2. **Distribution libraries** — reading and writing bundles and catalogs locally and remotely (e.g. OCI image registries, filesystems).
3. **Resolution** — resolving bundles from catalogs based on caller-specified constraints, including:
   - Dependency resolution (required/provided APIs, package dependencies)
   - Upgrade graph computation (valid upgrade paths between versions within a package/channel)
   - Catalog indexing and querying (lookup by package, channel, version, provided/required APIs)
4. **Validation** — validating bundle and catalog content for structural correctness, completeness, and policy compliance as part of parsing and loading.

## Non-Goals

- CLI tools or end-user applications
- Kubernetes cluster interactions — no kubeconfig, no kube client, no controller runtime
- Replacing operator-sdk or OLM itself
- Managing operator lifecycle (install, upgrade, uninstall) at the cluster level

## Design Principles

- **Pure data types with standalone functions** — keep types inert; transformations are functions that accept and return data, not methods that couple logic to types.
- **From/To naming convention** — conversion functions follow `FromX` / `ToY` patterns for discoverability and symmetry.
- **Internal packages for implementation details** — minimize public API surface; hide implementation behind `internal/` packages.
- **Minimize legacy dependencies** — `operator-framework/api` and `operator-framework/operator-registry` are legacy; depend on them only to the minimal extent necessary and prefer defining local types where practical.
- **No cluster dependencies** — nothing in this library should import client-go, controller-runtime, or assume access to a Kubernetes cluster.

## Development Practices

- All public API changes require tests.
- One logical change per commit.
- Work items progress through: idea → refined spec → implementation → verification.
- Keep PRs small and merge often.
