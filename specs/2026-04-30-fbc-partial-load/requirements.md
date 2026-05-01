# Requirements

- `fbc.PackageError` is a public type with `Package string` and `Errs []error` fields.
- `PackageError.Error()` produces a human-readable message including the package name and all error details.
- `PackageError.Unwrap() []error` returns the individual errors for `errors.Is`/`errors.As` compatibility.
- During ingest, channel and bundle blob failures (parse errors, version extraction errors) are collected per package name; the walk continues processing remaining blobs.
- During ingest, package blob parse failures (no package name available) and infrastructure errors (DB errors, context cancellation) remain fatal.
- During normalization, packages with ingest errors are skipped entirely.
- During normalization, handler errors for a package cause its transaction to be rolled back; remaining packages continue processing.
- `FromFS` merges ingest and normalization errors per package into `PackageError` values joined via `errors.Join`.
- `FromFS` returns a non-nil, queryable `Catalog` alongside the error when at least some packages loaded successfully.
- `FromFS` returns `(nil, err)` only for fatal errors unrelated to individual packages.
- Valid packages are fully queryable via `ListPackages`, `GetPackage`, etc. — failed packages are invisible to the catalog API.
- `GetPackage` for a failed package returns a not-found error (same as a package that never existed).

## Acceptance Criteria

- A catalog with one valid and one malformed package returns `(catalog, err)` where: the catalog lists only the valid package, and `err` contains a `PackageError` for the malformed package.
- A catalog where all packages are malformed returns `(catalog, err)` where: `ListPackages` yields zero results, and `err` contains a `PackageError` per package.
- A catalog with only valid packages returns `(catalog, nil)` — no behavior change.
- An empty catalog returns `(catalog, nil)` — no behavior change.
- The example program (`examples/query_operatorhubio`) loads the operatorhubio catalog, logs per-package warnings, and queries valid packages.
- Existing tests that expect fatal errors for single-package catalogs with bad data continue to pass (the single package fails, so the error is still returned — it's just structured differently).
