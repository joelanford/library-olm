---
status: idea
---
# Separate Reader/Writer Connection Pools for Content DB

## Problem

Both the content DB and staging DB use `MaxOpenConns(1)` to serialize access. This prevents nested read cursors — any code that opens a second query while a first is still open will deadlock waiting for the single connection.

This affects two areas today:

1. **Staging DB accessors**: `PackageAccessor.Bundles()`, `Channels()`, `Deprecations()`, `Others()`, and `channelAccessor.Entries()` previously streamed from live cursors. Nesting any of these (e.g., iterating bundles while also iterating channels in `FinalizePackage`) deadlocked. The current workaround is pre-collecting results into slices before yielding, which works but increases memory usage and prevents lazy evaluation.

2. **Content DB queries**: `BundleRow.Property()` and graph `Property()` queries execute additional reads against the same content DB. Calling them inside streaming iterators (`ListPackages`, `ListGraphs`, `ListBundles`, `Successors`) deadlocked when those iterators held open cursors. The current workaround is pre-collecting those iterator results before yielding.

## Idea

Open the same DB file twice — one `*sql.DB` with `MaxOpenConns(1)` for writes (`Set`, `Delete`), and another with unlimited connections for reads (`Get`, `List`, all query methods including `Bundle.Property()` and `UpdateGraph.Property()`). WAL mode makes this safe: readers don't block writers and vice versa.

This would let callers freely nest read iterators and call `Property()` inside iteration loops without deadlocking, while keeping write serialization simple.

Once implemented, the pre-collection pattern in both staging DB accessors (`collectChannels`, etc.) and content DB query iterators can be replaced with direct streaming, reducing memory overhead for large catalogs.

## Notes

- `BundleRow`, `UpdateGraphQuery`, etc. would hold a reference to the reader DB instead of the writer DB
- The staging DB could also benefit — remove `MaxOpenConns(1)` and let accessor iterators stream freely
- `modernc.org/sqlite` has had historical issues with `busy_timeout` and concurrent access — test thoroughly
- `BEGIN IMMEDIATE` may be needed for write transactions to avoid the deferred-transaction upgrade problem
