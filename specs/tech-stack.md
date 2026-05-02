# Tech Stack

## Language and Runtime

- **Language:** Go
- **Go version:** 1.25.7
- **Module path:** `github.com/joelanford/library-olm`

## Core Dependencies

| Dependency | Purpose |
|---|---|
| `helm.sh/helm/v4` | Helm chart parsing and handling |
| `go.podman.io/image/v5` | OCI image and registry access |
| `github.com/containerd/containerd` | Content store for image unpacking |
| `github.com/opencontainers/image-spec` | OCI image specification types |
| `github.com/opencontainers/go-digest` | Content-addressable digest handling |
| `github.com/cert-manager/cert-manager` | Certificate resource types for webhook cert provisioning |
| `github.com/santhosh-tekuri/jsonschema/v6` | JSON schema validation |
| `github.com/Masterminds/semver/v3` | Semantic version parsing and comparison |
| `github.com/blang/semver/v4` | Semantic version parsing (secondary) |
| `modernc.org/sqlite` | Pure-Go SQLite driver for catalog indexing |
| `golang.org/x/sync` | Concurrency utilities |

### Legacy Dependencies (minimize usage)

| Dependency | Purpose | Status |
|---|---|---|
| `github.com/operator-framework/api` | OLM API types (CSV, InstallMode, etc.) | Legacy — use minimally, prefer local types |
| `github.com/operator-framework/operator-registry` | Registry types | Legacy — use minimally |

## Dev/Test Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/stretchr/testify` | Test assertions (assert, require) |

## Project Structure

```
bundle/                          — bundle types and transformations
  v1/                            — bundle identity types (Release, VersionRelease, Bundle interface)
  registry/v1/                   — registry+v1 bundle parsing and rendering
    internal/                    — implementation details (render, validate, config)
  versionrelease.go              — version/release utilities
catalog/                         — catalog API, formats, and querying
  v1/                            — catalog interfaces (UpdateGraph, CompositeUpdateGraph, Catalog)
  fbc/                           — FBC catalog implementation (public: Catalog, FromFS, Close)
    internal/                    — SQLite schema, ingest, handler dispatch, query types
image/                           — OCI registry access and content unpacking
  bundle/                        — bundle image handlers (registry+v1, Helm)
  catalog/                       — catalog image handlers (FBC)
  internal/ociutil/              — shared manifest discovery and layer extraction
  internal/testutil/             — shared image test infrastructure
resolver/                        — bundle resolution from catalogs
specs/                           — project specs and work items
```

## Build and Test Commands

| Command | Purpose |
|---|---|
| `make ci` | Run the full CI check (lint, test, build) |
| `make lint` | Run golangci-lint |
| `make lint-fix` | Run golangci-lint with auto-fix |
| `make test` | Run all tests |
| `make build` | Build all packages |
| `make tidy` | Clean up module dependencies |

Linting is managed via `golangci-lint`, installed as a Go tool dependency (`go tool golangci-lint`). CI runs `make ci` on PRs and pushes to `main` via GitHub Actions.
