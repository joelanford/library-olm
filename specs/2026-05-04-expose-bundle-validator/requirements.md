# Requirements

- Export a `Validate(b Bundle) error` function from `bundle/registry/v1`.
- The function runs all internal bundle validation checks.
- Callers cannot configure or skip individual validators.
- Accept `Bundle` by value, consistent with the rest of the public API.

## Acceptance Criteria

- `registryv1.Validate(b)` is callable from external packages.
- It returns `nil` for valid bundles and a non-nil error describing failures for invalid bundles.
- The function delegates to the same validator set used by `ToPlainManifests`.
- No new public types or options are introduced for validation customization.
