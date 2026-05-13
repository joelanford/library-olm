# Requirements

## Properties storage

- `content_bundle_properties` table stores `(catalog_name, bundle_id, key, value JSON)` with primary key `(catalog_name, bundle_id, key)` and composite foreign key `(catalog_name, bundle_id) REFERENCES content_bundles(catalog_name, bundle_id) ON DELETE CASCADE`
- `content_graph_properties` table stores `(graph_id, key, value JSON)` with primary key `(graph_id, key)` and foreign key to `content_graphs(id) ON DELETE CASCADE` — `graph_id` is globally unique so no `catalog_name` column is needed; cascade delete handles cleanup when graphs are deleted
- Both tables are part of the content layer: created by `CreateContentTables`, dropped by `DropContentTables`, governed by `ContentSchemaVersion`
- `DeleteCatalogContent` does not need explicit property deletion — bundle properties cascade-delete via `content_bundles`, graph properties cascade-delete via `content_graphs`

## Writer interface

- `Writer` gains `SetBundleProperty(bundleID, key string, val any) error`
- `Writer` gains `SetGraphProperty(path []string, key string, val any) error`
- `ContentWriter` implements both by marshaling `val` via `json.Marshal` and inserting into the property tables within the existing transaction
- `writerAdapter` in `catalog/v1/db.go` delegates to `ContentWriter`

## Reading API

- `bundlev1.Bundle` gains `Property(ctx context.Context, key string) (json.RawMessage, error)`
- `catalogv1.UpdateGraph` gains `Property(ctx context.Context, key string) (json.RawMessage, error)`
- `BundleRow` implements `Property` by querying `content_bundle_properties`
- `UpdateGraphQuery` and `CompositeUpdateGraphQuery` implement `Property` by querying `content_graph_properties`
- `Property` returns `(nil, nil)` when the key is not found

## FBC Extension interface

- `fbc.OLMPackageExtension` interface with methods: `OnPackage(declcfg.Package) (any, error)`, `OnChannel(declcfg.Channel) (any, error)`, `OnBundle(declcfg.Bundle) (any, error)`, `OnDeprecation(declcfg.Deprecation) (any, error)`, `OnOther(declcfg.Meta) (any, error)`, `FinalizePackage(ctx context.Context, pkg PackageAccessor, w PropertyWriter) error`
- `fbc.WithOLMPackageExtension(ext OLMPackageExtension)` option on `NewImporter`
- Per-blob callback return values are marshaled via `json.Marshal` and stored in `ext_data JSON` columns on the corresponding staging tables
- `OnDeprecation` blobs are stored in a new `raw_olm_deprecation` staging table with columns `(package_name, ext_data JSON)`
- `OnOther` blobs are stored in a new `raw_other` staging table with columns `(schema, package_name, name, ext_data JSON)`

## PackageAccessor

- `PackageAccessor` interface: `Name() string`, `ExtData() (json.RawMessage, error)`, `Bundles() iter.Seq2[BundleAccessor, error]`, `Channels() iter.Seq2[ChannelAccessor, error]`, `Deprecations() iter.Seq2[DeprecationAccessor, error]`, `Others() iter.Seq2[OtherAccessor, error]`
- `BundleAccessor` interface: `Name() string`, `Package() string`, `Version() string`, `Release() string`, `Image() string`, `ExtData() json.RawMessage`
- `ChannelAccessor` interface: `Name() string`, `ExtData() json.RawMessage`, `Entries() iter.Seq2[ChannelEntryAccessor, error]`
- `ChannelEntryAccessor` interface: `BundleName() string`, `Replaces() string`, `Skips() []string`, `SkipRange() string`
- `DeprecationAccessor` interface: `ExtData() json.RawMessage`
- `OtherAccessor` interface: `Schema() string`, `Name() string`, `ExtData() json.RawMessage`
- The accessor queries staging tables scoped to the current package

## Import pipeline integration

- Per-blob callbacks run during the `WalkMetasFS` walk, after parsing into `declcfg` types
- `FinalizePackage` runs after normalization for each package, receiving a package-scoped `PropertyWriter`
- `fbc.PropertyWriter` interface: `SetBundleProperty(ctx, bundleName, key string, val any) error`, `SetGraphProperty(ctx context.Context, path []string, key string, val any) error`
- `PropertyWriter.SetGraphProperty` takes a path relative to the package root (e.g., `[]string{}` for the package graph, `[]string{"channelName"}` for a channel graph); the implementation prepends the package name before delegating to the underlying `Writer`
- `FinalizePackage` errors are collected per-package and merged into the `PartialImportError`
- Packages skipped due to ingest errors do not have `FinalizePackage` called
- When no OLMPackageExtension is registered, the import pipeline behaves identically to today

## Acceptance Criteria

- `Writer.SetBundleProperty` and `SetGraphProperty` persist data that is readable via `Bundle.Property` and `UpdateGraph.Property`
- Properties are deleted when `Store.Delete(name)` is called
- Properties are dropped and rebuilt when content schema version changes
- An FBC importer with `WithOLMPackageExtension` calls per-blob callbacks during walk and `FinalizePackage` after normalization
- Extension data returned from per-blob callbacks is available via `ExtData()` on the corresponding accessor
- `FinalizePackage` errors cause the package to appear in `PartialImportError` without affecting other packages
- An FBC importer without `WithOLMPackageExtension` behaves identically to the current implementation
- `make ci` passes (lint, test, build)
- New code has at least 70% statement coverage
