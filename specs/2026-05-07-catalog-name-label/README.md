---
status: pr-submitted
pr: https://github.com/joelanford/library-olm/pull/5
---
# Auto-inject Catalog Name Label

## Summary

Automatically inject the label `olm.operatorframework.io/metadata.name` with the catalog's name
as the value on every `Store.Set()` call. This ensures all catalogs in the store are selectable by
name via `Select()` without requiring callers to manage the label manually. If the caller provides
a conflicting value for this key via `WithLabels()`, `Set()` returns an error.

## Design

### Reserved label constant (`catalog/v1/`)

Unexported constant for the reserved label key:

```go
const labelCatalogName = "olm.operatorframework.io/metadata.name"
```

### Injection logic in `Set()`

The reserved label is always written to the `catalog_labels` table during `Set()`. Two cases:

**Caller provides `WithLabels()`:** Check if the caller's label map contains `labelCatalogName`
with a value different from the catalog name. If so, return an error. Then proceed with the
existing delete-and-reinsert flow, adding the reserved label to the set.

**Caller omits `WithLabels()`:** Upsert the reserved label via
`INSERT OR REPLACE INTO catalog_labels (catalog_name, key, value) VALUES (?, ?, ?)` so it's
always present, even if the caller only called `Set()` to update URI or priority.

### Effect on existing tests

Some existing tests assert `assert.Empty(t, cat.Labels())`. These will now return at minimum
`{labelCatalogName: "<name>"}`. Tests need updating to expect the reserved label.
