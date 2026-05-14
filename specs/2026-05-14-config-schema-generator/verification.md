# Verification

## Implementation Correctness

- [ ] `make generate` runs without errors and produces `registryv1bundleconfig.json`.
- [ ] Generated schema is byte-identical to the pre-existing file (no unintended changes on first run).
- [ ] `make verify` passes on a clean tree.
- [ ] `make verify` fails after manually editing `registryv1bundleconfig.json` (e.g., delete a line), and prints a clear error message.
- [ ] `make ci` runs `lint`, `verify`, `test`, `build` in order and passes.
- [ ] `go generate -tags containers_image_openpgp ./...` from the repo root works (the `//go:generate` directive is correctly placed and the tool resolves its inputs).
- [ ] The generator resolves `k8s.io/api` and `operator-framework/api` versions from `go.mod` (not hardcoded).
- [ ] No new module dependencies are added to `go.mod`.

## Project Conventions

- [ ] Commit message follows conventional commits format (`chore: add config schema generator`).
- [ ] No `//nolint` comments added.
- [ ] Generator code follows project design principles: standalone functions, clear naming.
- [ ] No unnecessary public API surface introduced (generator is under `internal/`).
- [ ] Existing tests still pass (`make test`).
- [ ] Lint passes (`make lint`).
