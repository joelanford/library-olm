---
status: done
pr: https://github.com/joelanford/library-olm/pull/10
---
# Deprecation Ingest

## Summary

Ingest OLM deprecation entries into the FBC staging database and propagate
deprecation messages into normalized content tables during the normalize phase.
This enables downstream consumers (in a future work item) to efficiently query
deprecation status from the content tables without relying on properties or
extension data.

The focus is purely on data ingest — exposing deprecation information to
consumers of the normalized content (public API, resolver, queries) is
explicitly out of scope and will be handled separately.

## Design

### Raw staging table

A new `raw_olm_deprecation_entries` table coexists alongside the existing
`raw_olm_deprecation` table. The existing table continues to serve the
OLMPackageExtension `ext_data` callback; the new table stores individual
deprecation entries for use during normalization.

```sql
CREATE TABLE raw_olm_deprecation_entries (
    package_name TEXT NOT NULL,
    schema       TEXT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    message      TEXT NOT NULL CHECK(length(message) > 0)
);
```

Columns map directly from `declcfg.DeprecationEntry`:
- `package_name` — from the parent `Deprecation.Package`
- `schema` — from `DeprecationEntry.Reference.Schema` (`olm.package`,
  `olm.channel`, or `olm.bundle`)
- `name` — from `DeprecationEntry.Reference.Name` (empty for package-level)
- `message` — from `DeprecationEntry.Message` (must be non-empty)

During ingest, `parseDeprecation` writes one row per entry in the deprecation's
`Entries` array. This is unconditional — it does not depend on whether an
`IngestExtension` is present (unlike the existing `raw_olm_deprecation` write).

### Content schema columns

A nullable `deprecation_message TEXT` column is added to both `content_graphs`
and `content_bundles`:

```sql
deprecation_message TEXT CHECK(deprecation_message IS NULL OR length(deprecation_message) > 0)
```

`NULL` means not deprecated. A non-empty string means deprecated with the given
message. Empty strings are rejected by the CHECK constraint.

`ContentSchemaVersion` is bumped from 4 to 5, triggering a full content rebuild
on next store open.

### Writer interface

Two new methods are added to `catalogv1.Writer`:

```go
SetGraphDeprecation(path []string, message string) error
SetBundleDeprecation(bundleID string, message string) error
```

The `ContentWriter` implementation uses UPDATE statements:

```sql
UPDATE content_graphs SET deprecation_message = ? WHERE catalog_name = ? AND path = ?
UPDATE content_bundles SET deprecation_message = ? WHERE catalog_name = ? AND bundle_id = ?
```

### Normalize flow

The `OLMPackageHandler.Normalize` method is extended to query
`raw_olm_deprecation_entries` and call the Writer's deprecation methods. After
creating graphs and inserting bundles, it:

1. Queries package-level deprecation (`schema = 'olm.package'`) and calls
   `w.SetGraphDeprecation(pkgPath, message)` if found
2. For each channel, queries channel-level deprecation
   (`schema = 'olm.channel' AND name = ?`) and calls
   `w.SetGraphDeprecation(chPath, message)` if found
3. Queries all bundle-level deprecations (`schema = 'olm.bundle'`) and calls
   `w.SetBundleDeprecation(name, message)` for each

No propagation: a package-level deprecation only sets the message on the
top-level graph, not on child graphs or bundles.

### No-op for extensions

The existing `raw_olm_deprecation` table and the `OnDeprecation` extension
callback are unchanged. Extensions continue to receive deprecation blobs and
store their own data in `ext_data`.
