# Implementation Plan

## 1. Enable foreign key enforcement and add property tables

- Add `PRAGMA foreign_keys = ON` to the pragma block in `OpenStore` (`catalog/v1/db.go`). This enables enforcement of all existing FK constraints (`content_graph_bundles`, `content_successors`, `content_predecessor_ranges`) and the new `ON DELETE CASCADE` on `content_graph_properties`
- Add `content_bundle_properties` and `content_graph_properties` table DDL to `contentSchemaSQL` in `catalog/v1/internal/content.go`. `content_bundle_properties` uses `FOREIGN KEY (catalog_name, bundle_id) REFERENCES content_bundles(catalog_name, bundle_id) ON DELETE CASCADE`. `content_graph_properties` uses `FOREIGN KEY (graph_id) REFERENCES content_graphs(id) ON DELETE CASCADE`
- Bump `ContentSchemaVersion` to 4
- No changes needed to `DeleteCatalogContent` — both property tables cascade-delete through their parent tables

## 2. Add Writer property methods

- Add `SetBundleProperty(bundleID, key string, val any) error` and `SetGraphProperty(path []string, key string, val any) error` to `ContentWriter` in `catalog/v1/internal/writer.go` — values are marshaled via `json.Marshal` before storage
- Add `SetBundleProperty(bundleID, key string, val any) error` and `SetGraphProperty(path []string, key string, val any) error` to the `Writer` interface in `catalog/v1/store.go`
- Update `writerAdapter` in `catalog/v1/db.go` to delegate the new methods to `ContentWriter`

## 3. Add property reading to Bundle

- Add `Property(ctx context.Context, key string) (json.RawMessage, error)` to the `bundlev1.Bundle` interface in `bundle/v1/bundle.go`
- Update `internal.BundleRow` to hold a reference to `*sql.DB` and `catalogName` so it can query `content_bundle_properties`
- Implement `Property` on `BundleRow`: query `content_bundle_properties WHERE catalog_name = ? AND bundle_id = ? AND key = ?`, return `(nil, nil)` on `sql.ErrNoRows`
- Update all `BundleRow` construction sites in `catalog/v1/internal/query.go` (`parseBundleRow` and callers) to pass the DB and catalog name


## 4. Add property reading to UpdateGraph

- Add `Property(ctx context.Context, key string) (json.RawMessage, error)` to the `catalogv1.UpdateGraph` interface in `catalog/v1/catalog.go`
- Implement on `internal.UpdateGraphQuery`: query `content_graph_properties WHERE graph_id = ? AND key = ?` — no struct changes needed since `GraphID` is already present
- Implement on `compositeUpdateGraphWrapper` in `catalog/v1/db.go` by delegating to the wrapped `CompositeUpdateGraphQuery`
- Implement on `internal.CompositeUpdateGraphQuery`: same query pattern — `GraphID` is already present

## 5. Add ext_data columns to FBC staging tables

- Add `ext_data JSON` column to `raw_olm_package`, `raw_olm_channel`, and `raw_olm_bundle` table DDL in `catalog/fbc/internal/db.go`
- Add `raw_olm_deprecation` staging table: `(package_name TEXT, ext_data JSON)`; add to `RawTables` list
- Add `raw_other` staging table: `(schema TEXT, package_name TEXT, name TEXT, ext_data JSON)`; add to `RawTables` list
- Update `parsePackage`, `parseChannel`, `parseBundle` in `catalog/fbc/internal/ingest.go` to accept and insert `ext_data` (NULL when no extension)

## 6. Define Extension interface and accessor types

- Create `catalog/fbc/extension.go` with:
  - `OLMPackageExtension` interface: `OnPackage`, `OnChannel`, `OnBundle`, `OnDeprecation`, `OnOther`, `FinalizePackage`
  - `PropertyWriter` interface: `SetBundleProperty(ctx, bundleName, key, val)`, `SetGraphProperty(ctx, path, key, val)` — path is relative to the package root
  - `PackageAccessor`, `BundleAccessor`, `ChannelAccessor` interfaces
- Add `WithOLMPackageExtension(ext OLMPackageExtension)` option type and plumb it through `Importer`

## 7. Integrate Extension callbacks into ingest

- Thread the OLMPackageExtension through `Ingest` (accept as optional parameter)
- In `WalkMetasFS` callback, after parsing each `declcfg` type, call the corresponding OLMPackageExtension callback
- Marshal the return value with `json.Marshal` and include as `ext_data` in the staging row insert
- For `declcfg.SchemaDeprecation`, parse into `declcfg.Deprecation`, call `OnDeprecation`, and insert into `raw_olm_deprecation`
- For unrecognized schemas (the `default:` case), call `OnOther` and insert into `raw_other`
- When no OLMPackageExtension is present, pass NULL for `ext_data` and skip `OnOther`

## 8. Implement PackageAccessor backed by staging DB

- Create `catalog/fbc/internal/accessor.go` with concrete types implementing `PackageAccessor`, `BundleAccessor`, `ChannelAccessor`, `DeprecationAccessor`, `OtherAccessor`
- Each accessor queries the staging tables filtered by package name, including `ext_data` columns
- `BundleAccessor` returns `Name()`, `Package()`, `Version()`, `Release()`, `Image()`, `ExtData()` from `raw_olm_bundle`
- `ChannelAccessor` returns `Name()`, `ExtData()` from `raw_olm_channel`. Also exposes channel entries (bundle name, replaces, skips, skipRange) so that `OLMPackageHandler.Normalize` can use the same accessor instead of raw SQL
- `DeprecationAccessor` returns `ExtData()` from `raw_olm_deprecation`
- `OtherAccessor` returns `Schema()`, `Name()`, `ExtData()` from `raw_other`
- `PackageAccessor.Deprecations()` and `PackageAccessor.Others()` query their respective tables filtered by package name
- `PackageAccessor.ExtData()` queries `raw_olm_package.ext_data` and returns `(json.RawMessage, error)`

## 9. Refactor OLMPackageHandler to use PackageAccessor

- Keep the `OLMPackageHandler.Normalize` signature as `(ctx, rawDB *sql.DB, w Writer, packageName string)` — the handler constructs a `PackageAccessor` internally via `NewPackageAccessor(rawDB, packageName)`
- Refactor `OLMPackageHandler.Normalize` to read bundles, channels, and channel entries from the `PackageAccessor` instead of raw SQL queries against staging tables
- Remove direct staging table queries from `OLMPackageHandler` (`validate`, `validateBundles`, `readChannels`, `readChannelEntries`, etc.) — all staging access goes through the accessor

## 10. Integrate FinalizePackage into import pipeline

- In `fbc.Importer.Import`, after the Normalize loop, add a second per-package loop that constructs a package-scoped `PropertyWriter` and calls `ext.FinalizePackage(ctx, accessor, propertyWriter)` for each package not in the skip set
- The `PropertyWriter` implementation (internal) holds the catalog name, package name, and the underlying `catalogv1.Writer`. It resolves: package name → graph ID (`parent_id IS NULL`), channel name → graph ID (`parent_id = packageGraphID`), bundle name → `(catalog_name, bundle_id)` in `content_bundles`
- Collect `FinalizePackage` errors per-package and merge them into the `PartialImportError` via `mergePackageErrors`
- Structure: `Import` calls `Ingest` → `Normalize` → `FinalizeExtension` (new), passing the extension, staging DB, content DB, catalog name, writer, and skip set

## 11. Tests

- `catalog/v1/internal` tests: property table creation, writer property methods, property reading on BundleRow and UpdateGraphQuery
- `catalog/v1` tests: end-to-end Set/Get with properties, Delete clears properties, content schema rebuild drops/recreates properties
- `catalog/fbc` tests: Extension callbacks receive correct declcfg types, ext_data is stored and accessible via PackageAccessor, FinalizePackage writes properties readable via Bundle.Property/UpdateGraph.Property, FinalizePackage errors appear in PartialImportError, no-extension import is unchanged
- Update `catalog/v1/db_fingerprint_test.go` to include property tables in the schema fingerprint
- Update any existing tests broken by the Bundle/UpdateGraph interface additions

## 12. Update content schema version fingerprint

- Update `db_fingerprint_test.go` golden data to reflect the new property tables and bumped schema version
