# library-olm

Go library providing reusable building blocks for OLM's core functionality: bundle and catalog definitions, transformations, distribution via OCI registries and filesystems, resolution, and validation. No Kubernetes cluster dependencies.

```
go get github.com/joelanford/library-olm
```

## Packages

### `bundle/registry/v1` — Registry+v1 bundles

Parse and render registry+v1 operator bundles.

```go
import v1 "github.com/joelanford/library-olm/bundle/registry/v1"

// Parse a bundle from a filesystem (e.g. os.DirFS or embed.FS)
bundle, err := v1.FromFS(bundleFS)

// Render to plain Kubernetes manifests
objects, err := v1.ToPlainManifests(bundle, "my-namespace",
    v1.WithTargetNamespaces("ns-a", "ns-b"),
    v1.WithCertificateProvider(certProvider),
    v1.WithDeploymentConfig(depConfig),
)
```

### `image/bundle` — Bundle image handlers

Unpack operator bundle images from OCI registries.

- **`RegistryV1Handler`** — unpacks registry+v1 bundle images (manifests + metadata directories)
- **`HelmChartHandler`** — unpacks Helm chart OCI artifacts with optional provenance verification

### `image/catalog` — Catalog image handlers

- **`FBCHandler`** — unpacks file-based catalog (FBC) images, with multi-platform manifest list support

## Design Principles

- **Pure data types with standalone functions** — types are inert; transformations are functions that accept and return data.
- **From/To naming convention** — conversion functions follow `FromX` / `ToY` for discoverability and symmetry.
- **Internal packages for implementation** — minimal public API surface; details hidden behind `internal/`.
- **No cluster dependencies** — nothing imports client-go or controller-runtime, or assumes access to a Kubernetes cluster.

## Development

```sh
make ci        # run the full CI check (lint, test, build)
make lint      # run golangci-lint
make lint-fix  # run golangci-lint with auto-fix
make test      # run all tests
make build     # build all packages
make tidy      # clean up dependencies
```

Requires Go 1.25.7+.
