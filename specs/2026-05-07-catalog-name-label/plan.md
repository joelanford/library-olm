# Implementation Plan

1. **Add `labelCatalogName` constant**
   - In `catalog/v1/store.go`, add `const labelCatalogName = "olm.operatorframework.io/metadata.name"`

2. **Inject the reserved label in `Set()`**
   - In `catalog/v1/db.go`, in the `Set` method:
     - Before the label-handling block, validate: if `cfg.labels` contains `labelCatalogName` with a value != `name`, return an error
     - In the `cfg.labels != nil` branch, add the reserved label to the map before inserting
     - After the existing label block (whether or not `cfg.labels` was set), upsert the reserved label: `INSERT OR REPLACE INTO catalog_labels (catalog_name, key, value) VALUES (?, ?, ?)`

3. **Update existing tests**
   - In `catalog/v1/db_test.go`:
     - `TestSet_NewWithURI`: change `assert.Empty(t, cat.Labels())` to expect `{labelCatalogName: name}`
     - `TestSet_Labels`: update assertions to include the reserved label alongside caller-provided labels
     - `TestSet_Labels` clear-labels case: expect the reserved label to remain even when clearing
   - In any other test files that assert on `Labels()`, update expectations

4. **Add new tests**
   - `TestSet_ReservedLabel_AutoInjected`: verify the label is present when no `WithLabels()` is passed
   - `TestSet_ReservedLabel_ConflictError`: verify `Set()` returns an error when `WithLabels()` contains a conflicting value
   - `TestSet_ReservedLabel_RedundantOK`: verify `Set()` succeeds when `WithLabels()` contains the matching value
   - `TestSet_ReservedLabel_PreservedOnUpdate`: verify the label persists on a metadata-only update (e.g., `WithPriority()` only)
   - `TestSelect_ByName`: verify `Select()` with `labelCatalogName=<name>` matches correctly
