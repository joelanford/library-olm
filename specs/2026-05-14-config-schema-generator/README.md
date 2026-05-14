---
status: pr-submitted
pr: https://github.com/joelanford/library-olm/pull/17
---
# Config Schema Generator

## Summary

Add a `go generate`-based tool to regenerate the embedded `registryv1bundleconfig.json` schema when the underlying Go types or Kubernetes API version change. This prevents the 2,600-line JSON schema from silently drifting from its source types. A `verify` Makefile target integrated into `ci` catches schema staleness in CI.

This connects to mission goal #1 (bundle definitions and transformations) — the config schema is how library-olm validates user configuration for registry+v1 bundles.

## Design

### Generator tool

A standalone Go program at `bundle/registry/v1/internal/bundle/gen-config-schema/main.go`, adapted from operator-controller's `hack/tools/schema-generator/main.go`. It:

1. Resolves the `k8s.io/api` version from `go.mod` (via `go list -m`) and converts it to a Kubernetes release tag (e.g., `v0.35.2` → `v1.35.2`).
2. Fetches the Kubernetes OpenAPI v3 spec for `api/v1` from the corresponding GitHub tag.
3. Parses `v1alpha1.SubscriptionConfig` from `operator-framework/api` (resolved via `go list -m`) to extract field names and types.
4. Generates the JSON schema with `$ref` links to the OpenAPI components for Kubernetes types.
5. Writes the output to `registryv1bundleconfig.json` in the same directory as the embed site.

### go:generate directive

A `//go:generate` comment is added to `bundle/registry/v1/internal/bundle/registryv1.go`, adjacent to the existing `//go:embed` directive. It invokes the generator with `go run` and the `containers_image_openpgp` build tag.

### Makefile targets

- `generate`: runs `go generate ./...` with the build tag.
- `verify`: runs `generate`, then checks `git diff --exit-code` (or `jj diff`) to ensure no uncommitted changes were produced. Fails if the schema is stale.
- `ci` target updated to: `ci: lint verify test build`.

### What stays the same

The runtime schema modification logic in `registryv1.go` (`buildBundleConfigSchema`, `buildWatchNamespaceProperty`, etc.) is unchanged. The generator only produces the base schema; the install-mode-specific adjustments still happen at runtime.
