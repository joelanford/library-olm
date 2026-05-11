# Verification

## Implementation Correctness

- [ ] `raw_olm_deprecation_entries` table exists in staging schema with
      correct columns and CHECK constraint on `message`
- [ ] `TableRawDeprecationEntries` constant and `RawTables` slice are updated
- [ ] `parseDeprecation` inserts into `raw_olm_deprecation_entries`
      unconditionally (not gated on `ext != nil`)
- [ ] `parseDeprecation` still inserts into `raw_olm_deprecation` only when
      `ext != nil` (existing behavior preserved)
- [ ] `content_graphs` has `deprecation_message TEXT` with CHECK constraint
- [ ] `content_bundles` has `deprecation_message TEXT` with CHECK constraint
- [ ] `ContentSchemaVersion` is 5
- [ ] `Writer` interface has both `SetGraphDeprecation` and
      `SetBundleDeprecation` methods
- [ ] `ContentWriter` implements both with UPDATE statements
- [ ] `writerAdapter` in `catalog/v1/db.go` delegates both new methods
- [ ] `OLMPackageHandler.Normalize` queries `raw_olm_deprecation_entries` and
      calls Writer deprecation methods for all three levels
- [ ] No propagation — package deprecation does not affect child graphs or
      bundles
- [ ] Test covers package-level, channel-level, and bundle-level deprecation
- [ ] Test covers non-deprecated entities retaining NULL
- [ ] Test covers empty message rejection (CHECK constraint)
- [ ] End-to-end test through Store.Set with FBC importer

## Project Conventions

- [ ] `make ci` passes (lint + test + build)
- [ ] No `//nolint` comments added
- [ ] Pure data types with standalone functions (mission.md design principle)
- [ ] No cluster dependencies introduced
- [ ] Legacy dependency usage not increased (declcfg types already in use)
- [ ] New code has at least 70% statement coverage
- [ ] Existing tests pass without modification
