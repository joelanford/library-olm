---
status: done
---
# Expose Bundle Validator as Public API

## Summary

Export a `Validate` function from the `bundle/registry/v1` package so callers can
validate a parsed `Bundle` independently of rendering. The library owns the
validation rules — callers cannot customize which validators run.

## Design

The internal `registryv1.BundleValidator` already aggregates all validation
checks (deployment uniqueness, CRD existence, webhook integrity, etc.) and
the `render.BundleValidator` type provides a `Validate` method. The public API
simply delegates to this existing machinery:

```go
func Validate(b Bundle) error
```

Key decisions:

- **Value receiver, not pointer** — consistent with `FromFS`, `ToPlainManifests`,
  and `ValidateConfig`, all of which accept `Bundle` by value.
- **No configuration** — the library is the sole opinion-holder on what
  constitutes a valid bundle. Callers get all-or-nothing validation.
- **Standalone function** — follows the project's "pure data types with
  standalone functions" design principle.
