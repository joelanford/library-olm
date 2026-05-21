# Implementation Plan

## Task Group 1: Create the generator tool

Adapt operator-controller's `hack/tools/schema-generator/main.go` into `bundle/registry/v1/internal/bundle/gen-config-schema/main.go`.

Changes from the original:
- Resolve the K8s OpenAPI spec URL automatically from `go.mod` (run `go list -m -f '{{.Version}}' k8s.io/api`, convert `v0.X.Y` → `v1.X.Y`, construct the GitHub raw URL).
- Resolve the `SubscriptionConfig` source file automatically from `go.mod` (run `go list -m -f '{{.Dir}}' github.com/operator-framework/api`, append `pkg/operators/v1alpha1/subscription_types.go`).
- Accept a single argument: the output file path. All other inputs are derived.
- Write output to the specified path.

## Task Group 2: Add go:generate directive

In `bundle/registry/v1/internal/bundle/registryv1.go`, add a `//go:generate` comment above the `//go:embed` line:

```go
//go:generate go run ./gen-config-schema registryv1bundleconfig.json
```

## Task Group 3: Add Makefile targets

Add `generate` and `verify` targets to the Makefile:

```makefile
generate:
	go generate -tags "$(GO_BUILD_TAGS)" ./...

verify: generate
	@if [ -n "$$(git diff --name-only)" ]; then \
		echo "ERROR: generated files are stale. Run 'make generate' and commit the result."; \
		git diff --stat; \
		exit 1; \
	fi
```

Update `ci` to: `ci: lint verify test build`.

Update `.PHONY` to include `generate` and `verify`.

## Task Group 4: Validate parity

Run `make generate` and confirm the output is identical to the existing `registryv1bundleconfig.json`. If there are differences, investigate and fix the generator until output matches exactly.

Run `make ci` end-to-end to confirm the full pipeline passes.
