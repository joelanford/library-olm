---
status: done
---
# FBC Partial Load with Per-Package Error Collection

## Summary

When loading an FBC catalog, a single malformed package currently causes the entire load to fail — both during ingest (channel/bundle parse failures, version extraction errors) and during normalization (skipRange parsing, successor computation). Instead, collect all per-package errors across both phases and return a working catalog containing only the valid packages, alongside a joined error describing all failures. Non-package errors (corrupted package blobs, DB failures, context cancellation) remain fatal.

## Design

### PackageError type (`catalog/fbc/`)

A public error type that aggregates all errors encountered for a single package across both ingest and normalization phases:

```go
// PackageError collects all errors encountered while loading a single
// package during catalog construction.
type PackageError struct {
    Package string
    Errs    []error
}

func (e *PackageError) Error() string { ... }
func (e *PackageError) Unwrap() []error { return e.Errs }
```

`Error()` formats as `package "<name>": <N> error(s): <joined details>`.

`Unwrap() []error` supports `errors.Is` / `errors.As` unwrapping into individual errors.

### Error collection flow

**Ingest phase** — `Ingest` currently fails fast via the `WalkMetasFS` callback. Change `metaToInsert` failures for channel and bundle blobs (which carry a package name) to record the error in a `map[string][]error` and return `nil` to the walker so it continues. Package blob parse failures (where no package name is available) remain fatal.

`Ingest` returns both the set of failed package names and the per-package errors so the caller can:
1. Pass the failed set to `Normalize` for exclusion.
2. Merge errors from both phases into the final `PackageError` values.

**Normalization phase** — `Normalize` skips packages in the failed set. For remaining packages, it wraps `handler.Normalize` errors in a collect-and-continue loop: on error, rollback the transaction and record it; on success, commit. Continue to the next package either way.

`Normalize` returns a `map[string][]error` of normalization failures.

**FromFS** — merges errors from both phases into a `[]PackageError` (one per failed package), joins them via `errors.Join`, and returns `(catalog, joinedErr)`. The catalog is always non-nil and queryable for valid packages when no fatal error occurs.

### Caller pattern

```go
cat, err := fbc.FromFS(ctx, fsys)
if err != nil {
    var pkgErr *fbc.PackageError
    for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
        if errors.As(e, &pkgErr) {
            log.Printf("WARNING: package %q: %v", pkgErr.Package, pkgErr)
        }
    }
    if cat == nil {
        return err // fatal error, no catalog
    }
}
defer cat.Close()
// use cat — only valid packages are queryable
```
