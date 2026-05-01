# Verification

## Implementation Correctness

- [ ] `PackageError` has public `Package` and `Errs` fields, `Error()` method, and `Unwrap() []error` method
- [ ] `Ingest` collects channel/bundle errors per package without aborting the walk
- [ ] `Ingest` still fails fast on package blob parse errors and infrastructure errors
- [ ] `Normalize` skips packages that had ingest errors
- [ ] `Normalize` rolls back failed packages and continues to the next
- [ ] `FromFS` merges errors from both phases into `PackageError` values
- [ ] `FromFS` returns `(catalog, joinedErr)` when some packages fail
- [ ] `FromFS` returns `(nil, err)` only for fatal errors
- [ ] Failed package data is cleaned from raw tables so queries don't return partial data
- [ ] Valid packages remain fully queryable after partial load
- [ ] `GetPackage` for a failed package returns not-found, not a partial result

## Test Coverage

- [ ] Single valid package — `(catalog, nil)`, no behavior change
- [ ] Single malformed package — `(catalog, err)`, catalog queryable but empty, error is `PackageError`
- [ ] Mixed valid + malformed — catalog contains only valid, error has `PackageError` for malformed
- [ ] All malformed — `(catalog, err)`, `ListPackages` yields zero
- [ ] Empty catalog — `(catalog, nil)`, no behavior change
- [ ] Ingest-phase error (bad version) captured in `PackageError`
- [ ] Normalize-phase error (bad skipRange) captured in `PackageError`
- [ ] `errors.As` can extract `PackageError` from the joined error
- [ ] Existing tests updated and passing

## Project Conventions

- [ ] Public API additions have tests (specs/conventions.md)
- [ ] `PackageError` is a standalone type, not a method on Catalog (specs/mission.md: pure data types)
- [ ] No cluster dependencies introduced (specs/mission.md)
- [ ] Legacy dependency usage not increased (specs/mission.md)
- [ ] Code builds: `make build`
- [ ] Lint passes: `make lint`
- [ ] Tests pass: `make test`
- [ ] Coverage not decreased
