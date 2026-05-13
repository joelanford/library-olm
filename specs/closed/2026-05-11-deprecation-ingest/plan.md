# Implementation Plan

## 1. Add raw_olm_deprecation_entries staging table

**Files:** `catalog/fbc/internal/db.go`

- Add `TableRawDeprecationEntries = "raw_olm_deprecation_entries"` constant
- Add it to the `RawTables` slice
- Add the CREATE TABLE statement to `rawSchemaSQL`:
  ```sql
  CREATE TABLE raw_olm_deprecation_entries (
      package_name TEXT NOT NULL,
      schema       TEXT NOT NULL,
      name         TEXT NOT NULL DEFAULT '',
      message      TEXT NOT NULL CHECK(length(message) > 0)
  );
  ```

## 2. Populate raw_olm_deprecation_entries during ingest

**Files:** `catalog/fbc/internal/ingest.go`

- Update `parseDeprecation` to always insert into `raw_olm_deprecation_entries`
  (one row per entry in `d.Entries`), regardless of whether `ext` is nil
- The existing `raw_olm_deprecation` insert remains conditional on `ext != nil`
- Both writes happen in the same transaction closure returned by
  `parseDeprecation`

## 3. Add deprecation_message column to content tables

**Files:** `catalog/v1/internal/content.go`

- Add `deprecation_message TEXT CHECK(deprecation_message IS NULL OR length(deprecation_message) > 0)`
  column to both `content_graphs` and `content_bundles` in `contentSchemaSQL`
- Bump `ContentSchemaVersion` from 4 to 5

## 4. Extend Writer interface and ContentWriter

**Files:** `catalog/v1/store.go`, `catalog/v1/internal/writer.go`, `catalog/v1/db.go`

- Add `SetGraphDeprecation(path []string, message string) error` and
  `SetBundleDeprecation(bundleID string, message string) error` to the
  `Writer` interface in `store.go`
- Implement both in `ContentWriter` (`internal/writer.go`):
  - `SetGraphDeprecation` resolves the graph path and UPDATEs
    `deprecation_message` on the matching `content_graphs` row
  - `SetBundleDeprecation` UPDATEs `deprecation_message` on the matching
    `content_bundles` row (by `catalog_name` and `bundle_id`)
- Add delegation methods to `writerAdapter` in `db.go` (the adapter that
  bridges the public `Writer` interface to the internal `ContentWriter`)

## 5. Update OLMPackageHandler.Normalize to write deprecation messages

**Files:** `catalog/fbc/internal/handler_olm_package.go`

- After creating all graphs and inserting all bundles (existing code), add a
  new step that queries `raw_olm_deprecation_entries` for the current package
- For `schema = 'olm.package'`: call `w.SetGraphDeprecation(pkgPath, message)`
- For each `schema = 'olm.channel'`: call
  `w.SetGraphDeprecation([]string{packageName, name}, message)`
- For each `schema = 'olm.bundle'`: call
  `w.SetBundleDeprecation(name, message)`

## 6. Add tests

**Files:** `catalog/fbc/internal/db_test.go` (or new test file)

- Test that `parseDeprecation` inserts entries into
  `raw_olm_deprecation_entries` even when `ext` is nil
- Test that the normalize flow sets deprecation messages on the correct
  content_graphs and content_bundles rows
- Test that non-deprecated entities retain NULL deprecation_message
- Use the existing `catalogfs.Builder` with its `WithDeprecation`,
  `PackageDeprecation`, `ChannelDeprecation`, and `BundleDeprecation` helpers
- End-to-end test via `Store.Set` with an FBC importer to verify the full
  pipeline including content schema version bump
