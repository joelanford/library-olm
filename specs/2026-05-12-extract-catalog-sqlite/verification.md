# Verification

## Implementation Correctness

### Package structure

- [ ] `catalog/v1/internal/` directory no longer exists
- [ ] `catalog/v1/db.go` no longer exists
- [ ] `catalog/v1/` contains only `catalog.go` and `store.go`
- [ ] `catalog/v1/sqlite/` exists with `sqlite.go`, `content.go`,
      `metadata.go`, `query.go`, `writer.go`
- [ ] `catalog/v1/sqlite` imports `catalog/v1` — not the reverse

### Deleted types and functions

- [ ] `writerAdapter` (type + 9 methods) — gone
- [ ] `compositeUpdateGraphWrapper` (type + 6 methods) — gone
- [ ] `wrapGraphNode`, `wrapGraphNodes` — gone
- [ ] `CatalogQuery` (type + 2 methods) — gone
- [ ] `GraphNode` type — gone
- [ ] `DeleteCatalogContent` standalone func — gone

### New structure

- [ ] `graphQuery` implements `catalogv1.UpdateGraph`
- [ ] `compositeGraphQuery` embeds `graphQuery`, implements
      `catalogv1.CompositeUpdateGraph`
- [ ] `queryGraphNodes` returns `iter.Seq2[catalogv1.UpdateGraph, error]`
- [ ] `queryGraphNode` (singular) exists for single-result lookups
- [ ] `contentWriter` directly implements `catalogv1.Writer`
- [ ] `contentWriter` has a `deleteAll()` method
- [ ] `storedCatalog` has `readerDB` field, calls query functions directly

### Invariants

- [ ] Type assertion `graph.(catalogv1.CompositeUpdateGraph)` fails for
      `*graphQuery`
- [ ] Type assertion `graph.(catalogv1.CompositeUpdateGraph)` succeeds
      for `*compositeGraphQuery`
- [ ] `sqlite.OpenStore` is the only exported symbol in `catalog/v1/sqlite`
- [ ] New exports in `catalog/v1` limited to `SetConfig`, `ContentConfig`,
      and `ApplySetOptions` (required for cross-package option resolution)

### Tests and callers

- [ ] All tests moved to `catalog/v1/sqlite/` or updated to import
      `sqlite.OpenStore`
- [ ] `examples/query_operatorhubio` updated
- [ ] `catalog/fbc/` tests updated
- [ ] `resolver/v1/` tests updated
- [ ] `go build ./...` succeeds
- [ ] All tests pass (`make test`)

## Project Conventions

- [ ] Commits follow conventional commits format (`refactor:`)
- [ ] `make lint` passes
- [ ] Design aligns with mission.md design principles
