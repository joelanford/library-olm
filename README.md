# library-olm

Go library providing reusable building blocks for OLM's core functionality: bundle and catalog definitions, transformations, distribution via OCI registries and filesystems, resolution, and validation. No Kubernetes cluster dependencies.

```
go get github.com/joelanford/library-olm
```

## Packages

| Package | Description |
|---|---|
| `bundle/v1` | Bundle identity types (`Bundle`, `NameVersionRelease`, `Release`) and version comparison |
| `bundle/registry/v1` | Parse registry+v1 bundles from a filesystem (`FromFS`) and render to plain Kubernetes manifests (`ToPlainManifests`) |
| `catalog/v1` | Persistent multi-catalog store (`OpenStore`, `Store`) with `Catalog`, `UpdateGraph`, `CompositeUpdateGraph` query interfaces and `Writer`/`Importer` for format-specific import |
| `catalog/fbc` | FBC importer (`NewImporter`) — imports File-Based Catalog data into a `catalog/v1` store |
| `image` | OCI registry access (`Repository`), caching (`CachingRepository`), and content-type-based unpacking (`Unpacker`) |
| `image/bundle` | Image handlers for registry+v1 bundles and Helm chart OCI artifacts |
| `image/catalog` | Image handler for file-based catalog (FBC) images with multi-platform support |

## Examples

Runnable examples are embedded as Go [testable examples](https://go.dev/blog/examples) — view them with `go doc` or browse the `example_test.go` files:

- [`bundle/v1`](bundle/v1/example_test.go) — parsing releases, comparing bundle identities
- [`bundle/registry/v1`](bundle/registry/v1/example_test.go) — parsing a bundle from a filesystem and rendering to plain Kubernetes manifests
- [`catalog/fbc`](catalog/fbc/example_test.go) — importing an FBC catalog into a store and querying packages, channels, bundles, and upgrade paths
- [`image`](image/example_test.go) — building a custom handler and unpacking OCI image content with filters

For a more complete end-to-end example that pulls a real catalog image from a registry:

- [`examples/query_operatorhubio`](examples/query_operatorhubio) — extract the OperatorHub.io catalog image, load it, and exercise the full query API

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
