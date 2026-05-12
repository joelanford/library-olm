# Requirements

- `catalog/v1` contains only public interfaces and option types — no
  implementation, no SQLite dependency
- All SQLite-backed implementation lives in `catalog/v1/sqlite`
- `catalog/v1/sqlite` imports `catalog/v1` — never the reverse
- `catalog/v1/internal` directory is deleted
- `sqlite.OpenStore` is the sole exported symbol in `catalog/v1/sqlite`
- No bridge types exist between packages — implementation types directly
  implement the public interfaces
- No behavioral changes — all queries, writes, schema migrations, and
  content operations work identically

## Acceptance Criteria

- `go build ./...` succeeds
- `go vet ./...` succeeds
- All existing tests pass (moved to new package or updated imports)
- `golangci-lint` passes
- `catalog/v1/` directory contains only `catalog.go` and `store.go`
- `catalog/v1/sqlite/` directory has no `internal/` sub-package
- The public API surface of `catalog/v1` is unchanged except for the
  removal of `OpenStore` (which moves to `sqlite.OpenStore`) and the
  addition of `SetConfig`, `ContentConfig`, and `ApplySetOptions`
  (needed for external `Store` implementations to resolve options)
