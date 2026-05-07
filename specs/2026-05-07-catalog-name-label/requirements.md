# Requirements

- Every catalog in the store has the label `olm.operatorframework.io/metadata.name` set to the catalog's name
- The label is injected automatically by `Set()` — callers do not need to include it in `WithLabels()`
- If a caller passes `WithLabels()` containing the reserved key with a value that differs from the catalog name, `Set()` returns an error and the catalog is not modified
- If a caller passes `WithLabels()` containing the reserved key with a value that matches the catalog name, `Set()` succeeds (no error for redundant-but-correct values)
- The reserved label key is defined as an unexported constant `labelCatalogName` in `catalog/v1/`

## Acceptance Criteria

- `Store.Set(ctx, "foo", WithURI("test://"))` produces a catalog with `Labels()` containing `{"olm.operatorframework.io/metadata.name": "foo"}`
- `Store.Set(ctx, "foo", WithURI("test://"), WithLabels(map[string]string{"env": "prod"}))` produces labels `{"olm.operatorframework.io/metadata.name": "foo", "env": "prod"}`
- `Store.Set(ctx, "foo", WithURI("test://"), WithLabels(map[string]string{"olm.operatorframework.io/metadata.name": "bar"}))` returns an error
- `Store.Set(ctx, "foo", WithURI("test://"), WithLabels(map[string]string{"olm.operatorframework.io/metadata.name": "foo"}))` succeeds
- Updating a catalog with `Set(ctx, "foo", WithPriority(5))` (no `WithLabels`) preserves the reserved label
- `Select()` with selector `olm.operatorframework.io/metadata.name=foo` matches the catalog named "foo"
- All existing tests pass after updating expectations to account for the reserved label
