---
status: in-progress
---
# App Properties and FBC Extension Hooks

## Summary

Enable third-party apps to attach custom metadata to catalog entities (bundles, graphs) through a first-class API without exposing library-olm's database internals. Properties are key-value pairs where values are `json.RawMessage`, stored in dedicated content tables and managed by library-olm's lifecycle (cleanup on delete, rebuild on schema change). An FBC Extension interface lets apps hook into the import pipeline with per-blob callbacks and a per-package finalize step.

## Problem

Apps like orb need to store additional metadata alongside catalog content (display names, descriptions, keywords, icons, related images). Today orb achieves this by directly opening library-olm's SQLite database file, creating its own side-channel tables, and managing their lifecycle independently. This couples orb to library-olm's storage implementation and requires orb to handle cleanup, rebuild, and transactional consistency on its own.

## Design

### Properties storage

Two new content tables, following the existing `content_*` naming convention:

```sql
CREATE TABLE content_bundle_properties (
    catalog_name TEXT NOT NULL,
    bundle_id    TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        JSON,
    PRIMARY KEY (catalog_name, bundle_id, key),
    FOREIGN KEY (catalog_name, bundle_id) REFERENCES content_bundles(catalog_name, bundle_id) ON DELETE CASCADE
);

CREATE TABLE content_graph_properties (
    graph_id INTEGER NOT NULL,
    key      TEXT NOT NULL,
    value    JSON,
    PRIMARY KEY (graph_id, key),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id) ON DELETE CASCADE
);
```

Properties live in the content layer: they are dropped and rebuilt on content schema version mismatch, and deleted when `Store.Delete(name)` is called. Graph properties use `ON DELETE CASCADE` so they are automatically cleaned up when the parent graph is deleted. This matches how orb's side-channel metadata is re-derived from the FBC source on each import.

`OpenStore` must enable `PRAGMA foreign_keys = ON` so cascades (and all existing FK constraints on `content_graph_bundles`, `content_successors`, `content_predecessor_ranges`) are enforced.

### Writer additions

The `Writer` interface gains two methods:

```go
SetBundleProperty(bundleID, key string, val any) error
SetGraphProperty(path []string, key string, val any) error
```

Values are marshaled via `json.Marshal` before storage, so callers can pass any JSON-serializable type. These are called during import (via FinalizePackage) to persist extension data alongside normalized content. The internal `ContentWriter` implements them by inserting into the property tables within the existing transaction.

### Reading API

The `bundlev1.Bundle` interface gains a `Property` method:

```go
type Bundle interface {
    BundleIdentity
    URI() string
    Property(ctx context.Context, key string) (json.RawMessage, error)
}
```

The `UpdateGraph` interface gains a `Property` method:

```go
type UpdateGraph interface {
    Name() string
    ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error]
    Successors(ctx context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error]
    Property(ctx context.Context, key string) (json.RawMessage, error)
}
```

The internal `BundleRow` gains a `*sql.DB` reference and catalog name to support lazy property loading from `content_bundle_properties`. `UpdateGraphQuery` and `CompositeUpdateGraphQuery` (which already hold `*sql.DB` and `GraphID`) query `content_graph_properties` directly — no additional fields needed since `graph_id` is globally unique. Properties not found return `(nil, nil)`.

### FBC Extension interface

An `OLMPackageExtension` interface in the `catalog/fbc` package hooks into the import pipeline. Registered via `fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(ext))`.

```go
type OLMPackageExtension interface {
    OnPackage(declcfg.Package) (any, error)
    OnChannel(declcfg.Channel) (any, error)
    OnBundle(declcfg.Bundle) (any, error)
    OnDeprecation(declcfg.Deprecation) (any, error)
    OnOther(declcfg.Meta) (any, error)
    FinalizePackage(ctx context.Context, pkg PackageAccessor, w PropertyWriter) error
}
```

**Per-blob callbacks** (`OnPackage`, `OnChannel`, `OnBundle`, `OnOther`): called during the FBC filesystem walk for each blob. The FBC importer already parses blobs into `declcfg` types; these are passed directly. The return value (`any`) is marshaled to JSON via `json.Marshal` and stored alongside the blob's staging row in an extra column (`ext_data JSON`). Return nil to store nothing.

- `OnPackage` receives the `declcfg.Package` that the ingest phase already parses. Its return value is stored in `raw_olm_package.ext_data`.
- `OnChannel` receives the `declcfg.Channel`. Stored in `raw_olm_channel.ext_data`.
- `OnBundle` receives the `declcfg.Bundle`. Stored in `raw_olm_bundle.ext_data`.
- `OnDeprecation` receives the `declcfg.Deprecation`. Stored in a new `raw_olm_deprecation` staging table with `ext_data`.
- `OnOther` receives the `declcfg.Meta` for blobs with unrecognized schemas (currently skipped by ingest). Stored in a new `raw_other` staging table with `ext_data`.

**FinalizePackage**: called once per package after normalization completes for that package. Receives a `PackageAccessor` — a read-only abstraction over the staging data for that package, including extension data from the per-blob callbacks. Writes properties via a `PropertyWriter` that is scoped to the current package and translates FBC names (bundle names, channel names) to content-layer identifiers internally.

If `FinalizePackage` returns an error, the package is added to the `PartialImportError` (same behavior as content normalization failures — valid packages are still imported).

```go
type PropertyWriter interface {
    SetBundleProperty(ctx context.Context, bundleName, key string, val any) error
    SetGraphProperty(ctx context.Context, path []string, key string, val any) error
}

type PackageAccessor interface {
    Name() string
    ExtData() (json.RawMessage, error)
    Bundles() iter.Seq2[BundleAccessor, error]
    Channels() iter.Seq2[ChannelAccessor, error]
    Deprecations() iter.Seq2[DeprecationAccessor, error]
    Others() iter.Seq2[OtherAccessor, error]
}

type BundleAccessor interface {
    Name() string
    Package() string
    Version() string
    Release() string
    Image() string
    ExtData() json.RawMessage
}

type ChannelAccessor interface {
    Name() string
    ExtData() json.RawMessage
    Entries() iter.Seq2[ChannelEntryAccessor, error]
}

type ChannelEntryAccessor interface {
    BundleName() string
    Replaces() string
    Skips() []string
    SkipRange() string
}

type DeprecationAccessor interface {
    ExtData() json.RawMessage
}

type OtherAccessor interface {
    Schema() string
    Name() string
    ExtData() json.RawMessage
}
```

The `PropertyWriter` implementation is internal to the FBC importer. It holds the package name and prepends it to graph paths before delegating to the underlying `Writer`:
- `SetGraphProperty([]string{}, key, val)` → `Writer.SetGraphProperty([]string{packageName}, key, val)` (package-level property)
- `SetGraphProperty([]string{"stable"}, key, val)` → `Writer.SetGraphProperty([]string{packageName, "stable"}, key, val)` (channel-level property)
- `SetBundleProperty(bundleName, key, val)` → `Writer.SetBundleProperty(bundleName, key, val)`

### Integration into import pipeline

The FBC importer's `Import` method changes as follows:

1. **Ingest phase** — `WalkMetasFS` already dispatches by schema. For each known schema, after parsing into `declcfg` types, call the Extension's corresponding callback. Marshal the result and include it in the staging row insert. For unknown schemas, call `OnOther` and insert into `raw_other`.
2. **Normalize phase** — unchanged. `OLMPackageHandler.Normalize` writes content through the Writer as before.
3. **New: Extension finalize phase** — after normalization for each package, construct a package-scoped `PropertyWriter` (backed by the same transaction used during normalization) and call `ext.FinalizePackage(ctx, accessor, propertyWriter)`. The accessor queries the staging tables (including `ext_data` columns). The `PropertyWriter` translates FBC names to content-layer identifiers and writes to the property tables directly. Errors are collected per-package.

### No app-ID namespace

Each app manages its own database file and importer configuration. There is no multi-tenant scenario where multiple apps share the same DB, so no app-ID namespace is needed on properties.

### Catalog-level properties

Deferred. Catalog-level properties can be added later if needed. The current design focuses on bundle and graph properties, which cover the orb use case.
