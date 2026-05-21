# Requirements

- The generator produces output byte-identical to the current `registryv1bundleconfig.json` (no regressions on initial run).
- The generator automatically resolves the correct Kubernetes OpenAPI spec version from `go.mod`'s `k8s.io/api` dependency.
- The generator automatically resolves the `SubscriptionConfig` source file from `go.mod`'s `operator-framework/api` dependency.
- Running `go generate ./...` with the `containers_image_openpgp` build tag regenerates the schema in place.
- `make verify` fails with a clear message if the checked-in schema doesn't match what the generator produces.
- `make ci` includes `verify` so CI catches schema drift.
- The generator has no dependencies beyond the project's existing `go.mod` (no new module dependencies).
- The generator runs without network access if the schema is already up to date (the verify step only needs to compare, not fetch).

## Acceptance Criteria

- `make generate` regenerates `registryv1bundleconfig.json` from source types.
- `make verify` passes on a clean checkout.
- `make verify` fails after manually editing `registryv1bundleconfig.json`.
- `make ci` runs lint, verify, test, and build in order.
- The generator's output matches the existing schema file exactly (initial parity check).
- The `//go:generate` directive in `registryv1.go` works with `go generate ./...`.
