---
status: done
pr: https://github.com/joelanford/library-olm/pull/11
---
# Extract catalog/v1 SQLite implementation

## Summary

Move the SQLite-backed `Store` implementation out of `catalog/v1` into
`catalog/v1/sqlite`. Today the public interfaces and their sole
implementation live in the same package, which forces implementation
details into an `internal/` sub-package and creates bridge types to
shuttle values across the boundary. Extracting the implementation into
its own package eliminates both problems: `catalog/v1` becomes a pure
interface package, and `catalog/v1/sqlite` contains the full
implementation with no internal sub-package needed.

## Design

### Package layout

```
catalog/v1/
  catalog.go       — UpdateGraph, CompositeUpdateGraph, Catalog interfaces
  store.go         — Store, StoreReader, Writer, Importer, SetOption, etc.
  sqlite/
    sqlite.go      — OpenStore (sole export), db, storedCatalog, selectedStore
    content.go     — content schema DDL, version checks
    metadata.go    — metadata migrations
    query.go       — graphQuery, compositeGraphQuery, bundleRow, query functions
    writer.go      — contentWriter (implements catalogv1.Writer), graphPath
  fbc/             — FBC importer (NewImporter → catalogv1.Importer)
    internal/      — raw table schema, ingest, handler dispatch
```

`catalog/v1/sqlite` imports `catalog/v1` for the interfaces.
`catalog/v1` imports nothing from `sqlite/`.

### What moves

`OpenStore` is the only export that moves. It becomes
`sqlite.OpenStore(path) → (catalogv1.Store, error)`.

Everything else in `catalog/v1` that is not part of the public interface
surface moves into `sqlite/` as unexported symbols:

| From | To |
|---|---|
| `catalog/v1/db.go` (implementation parts) | `catalog/v1/sqlite/sqlite.go` |
| `catalog/v1/internal/content.go` | `catalog/v1/sqlite/content.go` |
| `catalog/v1/internal/metadata.go` | `catalog/v1/sqlite/metadata.go` |
| `catalog/v1/internal/query.go` | `catalog/v1/sqlite/query.go` |
| `catalog/v1/internal/writer.go` | `catalog/v1/sqlite/writer.go` |

What stays in `catalog/v1`:

| File | Contents |
|---|---|
| `catalog.go` | `UpdateGraph`, `CompositeUpdateGraph`, `Catalog` interfaces |
| `store.go` | `Store`, `StoreReader`, `Writer`, `Importer`, `PartialImportError`, `SetOption`, `WithURI`, `WithPriority`, `WithLabels`, `WithContent` |

### Shared graph query base

`graphQuery` implements `catalogv1.UpdateGraph` for leaf graphs.
`compositeGraphQuery` embeds `graphQuery` and implements
`catalogv1.CompositeUpdateGraph` for composite graphs. Shared fields
and methods (`Name`, `Property`) live on the base; `compositeGraphQuery`
shadows `ListBundles`/`Successors` with descendant variants.

```go
type graphQuery struct {
    db          *sql.DB
    catalogName string
    graphID     int64
    graphName   string
}
// Name(), Property(), ListBundles(direct), Successors(direct)
// → implements catalogv1.UpdateGraph

type compositeGraphQuery struct {
    graphQuery
    graphPath []string
}
// ListBundles(descendant), Successors(descendant), ListGraphs(), GetGraph()
// → implements catalogv1.CompositeUpdateGraph
```

### queryGraphNodes returns UpdateGraph directly

With everything in the same package, `queryGraphNodes` constructs
`*graphQuery` or `*compositeGraphQuery` directly based on the
`HasChildren` scan result, returning `iter.Seq2[catalogv1.UpdateGraph, error]`.
A companion `queryGraphNode` (singular) handles single-result lookups.

This eliminates the `GraphNode` DTO, `wrapGraphNode`, and
`wrapGraphNodes` — all of which existed only to bridge the old
package boundary.

### contentWriter implements Writer directly

`contentWriter` methods already match the `catalogv1.Writer` interface
signatures. In the new package it implements the interface directly,
eliminating the `writerAdapter` bridge. `DeleteCatalogContent` folds
into `contentWriter` as a `deleteAll()` method.

### storedCatalog calls query functions directly

`storedCatalog` gains a `readerDB *sql.DB` field and calls
`queryGraphNodes`/`queryGraphNode` directly, eliminating the
`CatalogQuery` intermediary.

### Complete symbol disposition

#### catalog/v1/internal/content.go → sqlite/content.go

| Symbol | Kind | Disposition |
|---|---|---|
| `ContentSchemaVersion` | const | rename → `contentSchemaVersion` |
| `contentSchemaSQL` | const | no change |
| `CreateContentTables` | func | rename → `createContentTables` |
| `contentTablesDropOrder` | var | no change |
| `DropContentTables` | func | rename → `dropContentTables` |
| `CheckContentSchemaVersion` | func | rename → `checkContentSchemaVersion` |
| `StoreContentSchemaVersion` | func | rename → `storeContentSchemaVersion` |

#### catalog/v1/internal/metadata.go → sqlite/metadata.go

| Symbol | Kind | Disposition |
|---|---|---|
| `MetadataTables` | var | rename → `metadataTables` |
| `Migration` | type | rename → `migration` |
| `migration1SQL` | const | no change |
| `migration2SQL` | const | no change |
| `migration3SQL` | const | no change |
| `MetadataMigrations` | var | rename → `metadataMigrations` |
| `RunMetadataMigrations` | func | rename → `runMetadataMigrations` |
| `ClearAllDigests` | func | rename → `clearAllDigests` |

#### catalog/v1/internal/query.go → sqlite/query.go

| Symbol | Kind | Disposition |
|---|---|---|
| `tableExists` | func | no change |
| `BundleRow` | type | rename → `bundleRow` |
| `BundleRow.{ID,NVR,URI,Property}` | methods | no change (receiver rename handles it) |
| `GraphNode` | type | **delete** — `queryGraphNodes` returns `UpdateGraph` directly |
| `CatalogQuery` | type + 2 methods | **delete** — `storedCatalog` calls query functions directly |
| `UpdateGraphQuery` | type | **replace** with `graphQuery` (shared base) |
| `UpdateGraphQuery.{Name,Property}` | methods | move to `graphQuery` |
| `UpdateGraphQuery.{ListBundles,Successors}` | methods | move to `graphQuery` (direct variants) |
| `CompositeUpdateGraphQuery` | type | **replace** with `compositeGraphQuery` embedding `graphQuery` |
| `CompositeUpdateGraphQuery.{Name,Property}` | methods | **delete** — inherited from `graphQuery` |
| `CompositeUpdateGraphQuery.{ListBundles,Successors}` | methods | shadow on `compositeGraphQuery` (descendant variants) |
| `CompositeUpdateGraphQuery.{ListGraphs,GetGraph}` | methods | stay on `compositeGraphQuery` |
| `queryGraphProperty` | func | no change |
| `queryGraphNodes` | func | **change** — returns `iter.Seq2[catalogv1.UpdateGraph, error]` |
| `queryBundlesDirect` | func | no change |
| `queryBundlesDescendant` | func | no change |
| `streamBundleRows` | func | no change |
| `querySuccessorsDirect` | func | no change |
| `querySuccessorsDescendant` | func | no change |
| `querySuccessorsStreaming` | func | no change |
| `parseBundleRow` | func | no change |

#### catalog/v1/internal/writer.go → sqlite/writer.go

| Symbol | Kind | Disposition |
|---|---|---|
| `graphPath` | func | no change |
| `ContentWriter` | type | rename → `contentWriter` |
| `NewContentWriter` | func | rename → `newContentWriter` |
| `ContentWriter.{all methods}` | methods | no change (receiver rename handles it) |
| `DeleteCatalogContent` | func | **fold** into `contentWriter` as `deleteAll()` method |

#### catalog/v1/db.go → sqlite/sqlite.go

| Symbol | Kind | Disposition |
|---|---|---|
| `OpenStore` | func | move to `sqlite.OpenStore` (stays exported) |
| `db` | type | move (stays unexported) |
| `sqliteDSN` | func | move (stays unexported) |
| `storedCatalog` | type + methods | move, gains `readerDB` field |
| `selectedStore` | type + methods | move (unchanged) |
| `andSelector` | func | move (unchanged) |
| `querier` | interface | move (unchanged) |
| `getCatalog` | func | move (unchanged) |
| `queryLabels` | func | move (unchanged) |
| `labelCatalogName` | const | move from `store.go` (only used by implementation) |
| `writerAdapter` | type + 9 methods | **delete** |
| `compositeUpdateGraphWrapper` | type + 6 methods | **delete** |
| `wrapGraphNode` | func | **delete** |
| `wrapGraphNodes` | func | **delete** |

### What stays in catalog/v1

After extraction, `catalog/v1/catalog.go` and `catalog/v1/store.go`
contain only interfaces, option types, and option constructors. No
implementation types, no SQLite dependency, no `internal/` directory.

### Public API change

`catalogv1.OpenStore` moves to `sqlite.OpenStore`. `SetConfig`,
`ContentConfig`, and `ApplySetOptions` are exported so that external
`Store` implementations can resolve functional options. All interfaces
and option types remain in `catalog/v1`. Callers that construct stores
add an import:

```go
import (
    catalogv1 "github.com/joelanford/library-olm/catalog/v1"
    "github.com/joelanford/library-olm/catalog/v1/sqlite"
)

store, err := sqlite.OpenStore(path)
```

Callers that only consume interfaces (`catalogv1.Store`, `catalogv1.Catalog`,
etc.) are unaffected.
