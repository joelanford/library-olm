# library-olm

Go library providing reusable building blocks for OLM's core functionality: bundle and catalog definitions, transformations, distribution (OCI registries, filesystems), resolution, and validation. No Kubernetes cluster dependencies.

## Architecture

```
bundle/              — bundle types and transformations
  v1/                — bundle identity types (BundleID, NameVersionRelease, Bundle interface)
  registry/v1/       — registry+v1 bundle parsing (FromFS) and rendering (ToPlainManifests)
catalog/             — catalog API, persistent store, formats, and querying
  v1/                — catalog interfaces (Catalog, Store, Writer, Importer)
    sqlite/          — SQLite-backed Store implementation (OpenStore)
    fbc/             — FBC importer (NewImporter → catalogv1.Importer)
image/               — OCI registry access and content unpacking
  bundle/            — bundle image handlers (registry+v1, Helm)
  catalog/           — catalog image handlers (FBC)
  internal/ociutil/  — shared manifest discovery and layer extraction utilities
resolver/            — bundle resolution from catalogs
```

## Design Principles

- Pure data types with standalone functions (not methods coupling logic to data)
- From/To naming convention for conversions
- Internal packages for implementation details; minimal public API surface
- Minimize legacy dependency usage (operator-framework/api, operator-framework/operator-registry)
- No cluster dependencies (no kubeconfig, no kube client, no controller-runtime)

## Build and Test

```
make ci        # run the full CI check (lint, test, build)
make lint      # run golangci-lint
make lint-fix  # run golangci-lint with auto-fix
make test      # run all tests
make build     # build all packages
make tidy      # clean up dependencies
```

## Conventions

- **Commits:** conventional commits (`feat:`, `fix:`, `refactor:`, etc.), imperative mood, max 72 chars
- **PRs:** conventional commit title, body with Summary + Test plan sections
- **Branches:** `YYYY-MM-DD-<slug>` for spec work, `<slug>` for ad-hoc

See `specs/conventions.md` for full details.

## Workflow Commands

Work items live in `specs/YYYY-MM-DD-<slug>/` with frontmatter status tracking in README.md (`idea` → `ready` → `in-progress` → `pr-submitted` → `done`).

Ideas have only a `README.md`. Refined work items have four files:

| File | Purpose |
|---|---|
| `README.md` | High-level summary, design decisions, and status frontmatter |
| `requirements.md` | Functional requirements and acceptance criteria |
| `plan.md` | Specific implementation plan with ordered task groups |
| `verification.md` | How to verify the implementation is correct and follows project conventions |

| Command | Purpose |
|---|---|
| `/sdd-quick-item` | Quickly capture an idea to the backlog |
| `/sdd-ideate` | Brainstorm new work items and refine existing ideas |
| `/sdd-plan-next-phase` | Pick and fully refine the next work item |
| `/sdd-implement` | Implement a refined work item from its spec |
| `/sdd-review` | Review changes for correctness and consistency |
| `/sdd-ship` | Verify, commit, push, create PR, and monitor CI |

## Governing Specs

- `specs/mission.md` — goals, non-goals, design principles
- `specs/tech-stack.md` — language, dependencies, project structure, build commands
- `specs/conventions.md` — commit, PR, and branch conventions
