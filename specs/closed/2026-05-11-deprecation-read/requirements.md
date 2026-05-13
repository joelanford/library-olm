# Requirements

- A `Deprecated` interface with `DeprecationMessage() string` is defined in
  `catalog/v1/catalog.go`
- An unexported `deprecation` struct implements the interface once; all
  wrapper types embed it
- `Bundle`, `UpdateGraph`, and `CompositeUpdateGraph` interfaces are unchanged
- When a graph has a non-NULL `deprecation_message` in `content_graphs`, the
  returned `UpdateGraph` value also implements `Deprecated`
- When a bundle has a non-NULL `deprecation_message` in `content_bundles`,
  the returned `Bundle` value also implements `Deprecated`
- Non-deprecated graphs and bundles do NOT implement `Deprecated`
- Wrapping preserves all existing interface behavior (`ListBundles`,
  `Successors`, `Property`, `ListGraphs`, `GetGraph`, etc.)

## Acceptance Criteria

- Type assertion `graph.(catalogv1.Deprecated)` succeeds for deprecated
  packages and channels, and returns the correct message
- Type assertion `bundle.(catalogv1.Deprecated)` succeeds for deprecated
  bundles, and returns the correct message
- Type assertion fails for non-deprecated graphs and bundles
- Deprecated composite graphs still satisfy `CompositeUpdateGraph`
- All existing tests pass without modification
- End-to-end test through `Store.Set` and catalog query verifies deprecation
  is readable
