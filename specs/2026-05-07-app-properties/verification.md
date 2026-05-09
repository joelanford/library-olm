# Verification

## Implementation Correctness

- [ ] `PRAGMA foreign_keys = ON` is set in `OpenStore`, enforcing all FK constraints including cascades
- [ ] `content_bundle_properties` and `content_graph_properties` tables are created by `CreateContentTables` and dropped by `DropContentTables`
- [ ] `content_bundle_properties` uses `ON DELETE CASCADE` via composite FK to `content_bundles(catalog_name, bundle_id)` — deleting a bundle automatically deletes its properties
- [ ] `content_graph_properties` uses `ON DELETE CASCADE` via FK to `content_graphs(id)` — deleting a graph automatically deletes its properties
- [ ] `ContentSchemaVersion` is bumped (to 4), triggering content rebuild on existing databases
- [ ] `DeleteCatalogContent` requires no explicit property deletion — both cascade through their parent tables
- [ ] `Writer.SetBundleProperty` and `SetGraphProperty` are implemented on `ContentWriter`, `writerAdapter`, and the `Writer` interface
- [ ] `Bundle.Property(ctx, key)` returns the stored `json.RawMessage` or `(nil, nil)` when not found
- [ ] `UpdateGraph.Property(ctx, key)` returns the stored `json.RawMessage` or `(nil, nil)` when not found
- [ ] Staging tables have `ext_data JSON` columns; `raw_other` table exists
- [ ] `Extension.OnPackage/OnChannel/OnBundle/OnDeprecation/OnOther` callbacks are called with correct `declcfg` types during ingest
- [ ] Callback return values are marshaled and stored in `ext_data` staging columns
- [ ] `PackageAccessor`, `BundleAccessor`, `ChannelAccessor` return correct staging data including `ExtData()`
- [ ] `FinalizePackage` is called once per package after normalization with a package-scoped `PropertyWriter`
- [ ] `PropertyWriter.SetGraphProperty(ctx, []string{}, key, val)` writes to `content_graph_properties` for the package's top-level graph
- [ ] `PropertyWriter.SetGraphProperty(ctx, []string{"channelName"}, key, val)` writes to `content_graph_properties` for the named channel's graph
- [ ] `PropertyWriter.SetBundleProperty` writes to `content_bundle_properties` for the named bundle
- [ ] `FinalizePackage` errors are per-package and appear in `PartialImportError`
- [ ] Packages skipped due to ingest errors do not have `FinalizePackage` called
- [ ] `OLMPackageHandler.Normalize` uses `PackageAccessor` instead of direct staging table SQL — no raw SQL queries remain in the handler
- [ ] Import without `WithOLMPackageExtension` behaves identically to the current implementation (no regressions)
- [ ] End-to-end: Extension writes properties during FinalizePackage that are readable via `Bundle.Property` and `UpdateGraph.Property` after `Store.Set` completes

## Project Conventions

- [ ] Commit messages use conventional commit format (`feat:`, `refactor:`, etc.)
- [ ] One logical change per commit
- [ ] No `//nolint` comments added
- [ ] Public API additions (`Writer`, `Bundle`, `UpdateGraph`, `Extension`, accessor types) have tests
- [ ] New code has at least 70% statement coverage
- [ ] Pure data types with standalone functions — Extension is an interface, not a method on Importer internals
- [ ] From/To naming convention followed where applicable
- [ ] Implementation details in `internal/` packages — accessor implementations, staging queries
- [ ] No legacy dependency usage introduced beyond what exists
- [ ] No cluster dependencies introduced
- [ ] `make ci` passes (lint, test, build)
- [ ] `db_fingerprint_test.go` golden data updated for new tables and schema version
