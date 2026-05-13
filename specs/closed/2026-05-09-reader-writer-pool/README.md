---
status: done
pr: https://github.com/joelanford/library-olm/pull/9
---
# Separate Reader/Writer Connection Pools

## Summary

Split both the content DB and the FBC importer's staging DB from a single `*sql.DB` with
`MaxOpenConns(1)` into separate reader and writer pools sharing the same file. The writer pool
keeps `MaxOpenConns(1)` for write serialization. The reader pool allows unlimited concurrent
connections. WAL mode makes this safe: readers never block writers and vice versa.

This eliminates the nested-query deadlock that today forces all iterators to pre-collect results
into slices before yielding. With multiple reader connections, iterators can stream rows directly,
reducing memory overhead for large catalogs and enabling lazy evaluation.

## Design

### The two databases

There are two separate SQLite databases involved:

**Content DB** (`catalog/v1/`) — the persistent, long-lived database opened via `OpenStore`.
It stores catalog metadata (name, URI, digest, priority, labels) and normalized content
(bundles, graphs, successor edges, properties). The `Store` interface reads and writes this
database. Query types (`CatalogQuery`, `UpdateGraphQuery`, `BundleRow`, etc.) read from it.

**FBC staging DB** (`catalog/fbc/internal/`) — a temporary SQLite database that the FBC
importer creates to stage raw FBC data during import. This is an implementation detail of the
FBC importer, not a general catalog concept — other importers could stage data differently or
not at all. During ingest, raw FBC blobs (packages, channels, bundles, deprecations) are
written into staging tables. During normalize and finalize, `PackageAccessor` reads from
the staging tables and the `OLMPackageHandler` transforms the raw data into normalized
content, writing results through a `catalogv1.Writer` that targets the content DB. The
staging DB is deleted after import completes.

Both databases today use `MaxOpenConns(1)`, which prevents nested read cursors and forces
pre-collection workarounds. Both get the same reader/writer split.

### Two-pool pattern

For each database, open the same file twice — one `*sql.DB` for writes, one for reads:

| Pool | MaxOpenConns | Used by |
|------|-------------|---------|
| Writer | 1 | Content: `Set`, `Delete`, migrations, schema rebuilds. Staging: ingest |
| Reader | unlimited | Content: `Get`, `List`, `Select`, all query/iterator methods, `Property()`. Staging: normalize, finalize |

WAL mode (already enabled) guarantees readers and writers don't block each other. The writer
pool's single-connection limit serializes all write transactions. The reader pool's unlimited
connections allow nested iteration and `Property()` calls inside iterator loops.

### Per-connection pragma initialization

With `MaxOpenConns(1)`, setting pragmas once via `Exec` works because there's only one connection.
The reader pool creates connections on demand, so pragmas must be set per-connection. Both pools
use DSN `_pragma` parameters for consistency. The `_pragma` DSN parameter is supported by
`modernc.org/sqlite`:

Reader pool DSN:
```
file:<path>?mode=ro&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)
```

Writer pool DSN:
```
file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)
```

The reader pool's `mode=ro` flag enforces read-only access at the SQLite level — any accidental
write through the reader pool returns an error rather than silently succeeding. The writer pool
omits `mode=ro` to allow writes.

The path must be properly escaped if it contains URI-special characters (`?`, `#`, `%`). In
practice, database file paths rarely contain these characters.

### Content DB changes (`catalog/v1/`)

The `db` struct gains a second field:

```go
type db struct {
    writerDB *sql.DB
    readerDB *sql.DB
}
```

`OpenStore` opens the file twice. Writes (`Set`, `Delete`, migrations, schema rebuilds) use
`writerDB`. Reads (`Get`, `List`, query construction) use `readerDB`. Startup schema checks
run on the writer pool since they execute before the reader pool is needed and may trigger
a rebuild. `Close` closes both.

All internal query types (`CatalogQuery`, `CompositeUpdateGraphQuery`, `UpdateGraphQuery`,
`BundleRow`) already store a `DB *sql.DB` field used only for reads. After the split, they
receive `readerDB` instead of the single pool — no structural change to these types.

With the reader pool in place, all `collect*` pre-collection functions in `query.go` can be
replaced with direct streaming iterators. The `db.List()` method in `db.go` can also stream
catalog metadata rows directly instead of pre-collecting.

### Unified graph node listing

`ListPackages`, `GetPackage`, `ListGraphs`, and `GetGraph` all query the same
`content_graphs` table with different WHERE conditions. A single `queryGraphNodes` helper
builds the SQL from optional `parentID` and `name` parameters, returning `GraphNode` values
that carry `HasChildren` (via an `EXISTS` subquery) and `Path` (the full hierarchy of graph
names from root to node). The `db.go` layer wraps each `GraphNode` as either a
`compositeUpdateGraphWrapper` or `*UpdateGraphQuery` based on `HasChildren`.

This removes the prior assumptions that packages are always composite and child graphs are
always leaf nodes — both can be either.

### TOCTOU fix in Set

`Set` reads catalog metadata and labels from the write transaction before committing, then
returns the result. Previously it committed first and called `Get` via the reader pool,
which could return a stale or inconsistent result if another writer modified the catalog
between commit and read.

A `querier` interface (satisfied by both `*sql.DB` and `*sql.Tx`) enables shared
`getCatalog` and `queryLabels` helpers used by both `Set` (via `tx`) and `Get` (via
`readerDB`).

### FBC staging DB changes (`catalog/fbc/`)

`OpenTempDB` returns both a writer and reader pool. The importer passes the writer to `Ingest`
and the reader to `Normalize` and `finalize`. `CloseTempDB` closes both pools.

`PackageAccessor` and `channelAccessor` store the reader pool. All `collect*` pre-collection
functions in `accessor.go` can be replaced with direct streaming.

### Successor query streaming

The successor queries (`collectSuccessorResults`) union explicit edges with range-based matches
and deduplicate by bundle ID. With streaming, this becomes:

1. Stream explicit successor query, yield results, track seen IDs in a map
2. Stream range successor query, skip already-seen IDs, yield remaining results

Both queries use the reader pool and don't conflict. The `seen` map lives in the iterator closure.
