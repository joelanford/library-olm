# Requirements

- The FBC staging schema includes a `raw_olm_deprecation_entries` table with
  columns `package_name`, `schema`, `name`, and `message`
- The `message` column has a CHECK constraint enforcing `length(message) > 0`
- During ingest, each `olm.deprecations` blob produces one row per entry in
  the `Entries` array, regardless of whether an `IngestExtension` is present
- The existing `raw_olm_deprecation` table and `OnDeprecation` callback are
  unchanged
- `content_graphs` and `content_bundles` each have a nullable
  `deprecation_message TEXT` column with
  `CHECK(deprecation_message IS NULL OR length(deprecation_message) > 0)`
- `ContentSchemaVersion` is bumped from 4 to 5
- `catalogv1.Writer` has `SetGraphDeprecation(path []string, message string) error`
  and `SetBundleDeprecation(bundleID string, message string) error` methods
- `ContentWriter` implements both methods using UPDATE statements
- `OLMPackageHandler.Normalize` queries `raw_olm_deprecation_entries` and
  writes deprecation messages via the Writer for package graphs, channel
  graphs, and bundles
- Deprecation is stored exactly as declared — no propagation from package to
  channels or bundles

## Acceptance Criteria

- A catalog with `olm.deprecations` blobs populates `raw_olm_deprecation_entries`
  with one row per deprecation entry
- After normalization, `content_graphs.deprecation_message` is set for
  deprecated packages and channels
- After normalization, `content_bundles.deprecation_message` is set for
  deprecated bundles
- Non-deprecated graphs and bundles have `NULL` deprecation_message
- An empty-string deprecation message is rejected (CHECK constraint)
- Existing tests continue to pass without modification (the new column
  defaults to NULL)
- `ContentSchemaVersion` bump triggers content rebuild on existing databases
