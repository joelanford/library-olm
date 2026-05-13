# Requirements

- Provide a `FromFS(ctx, fs.FS) (*Catalog, error)` constructor that loads FBC data into a SQLite-backed catalog.
- Two-phase architecture: phase 1 ingests raw blobs into raw tables; phase 2 dispatches per-package-schema handlers that validate and populate normalized tables.
- During the ingest phase, keep only one parsed blob in memory at a time — write structured fields to the database immediately.
- Parse `olm.package`, `olm.channel`, and `olm.bundle` schemas during ingest; ignore unknown schemas.
- Extract bundle version from the `olm.package` property on each `olm.bundle` blob using `blang/semver/v4`.
- Define a `PackageSchemaHandler` interface (`Schema()` and `Normalize(ctx, tx, packageName)`) that handlers implement per package schema type. Each package's normalization receives its own `*sql.Tx`.
- Register a handler for `olm.package` that knows its companion schemas (`olm.channel`, `olm.bundle`) and normalizes them.
- During normalization, the `olm.package` handler validates semantic constraints: known references, no duplicates, all channel entries have corresponding bundle blobs.
- During normalization, the `olm.package` handler computes successor edges from `replaces`, `skips`, and `skipRange` fields and writes them to the normalized `successors` table.
- Return validation and normalization errors from `FromFS`, not deferred to query time.
- Implement `catalogv1.Catalog` — `ListPackages` and `GetPackage` return `catalogv1.CompositeUpdateGraph`s.
- Implement `catalogv1.CompositeUpdateGraph` — `ListGraphs`/`GetGraph` return per-channel `catalogv1.UpdateGraph`s.
- Implement `catalogv1.UpdateGraph` — `ListBundles` yields bundles in the graph, `Successors` yields successor bundles from precomputed edges.
- `ListBundles` on a `CompositeUpdateGraph` (package level) yields the deduplicated union across all graphs.
- `Successors` on a `CompositeUpdateGraph` (package level) yields the deduplicated union across all graphs.
- Bundle values yielded by iterators are `bundlev1.NameVersionRelease` (identity only).
- The SQLite database is stored in a temporary file. `Close()` releases resources and removes the file.
- Use `modernc.org/sqlite` (pure Go, no cgo) as the SQLite driver.
- Reuse `operator-framework/operator-registry/alpha/declcfg` for FBC parsing (`WalkMetasFS`, `Meta`, schema constants, typed blob unmarshaling).
- All database access, schema definitions, handler interface, handler implementations, and query types live in `catalog/fbc/internal/`.
- The public `catalog/fbc/` package exports only `Catalog`, `FromFS`, and `Close`.

## Acceptance Criteria

- `FromFS` loads a valid FBC filesystem and returns a `*Catalog` that satisfies `catalogv1.Catalog`.
- `GetPackage` returns a `catalogv1.CompositeUpdateGraph` with channel-level child graphs.
- `ListBundles` and `Successors` return correct results at both package and channel levels.
- `FromFS` returns an error for malformed or semantically invalid FBC data.
- Memory usage during ingest is proportional to a single FBC blob, not the entire catalog.
- The `PackageSchemaHandler` interface is defined and the `olm.package` handler is registered.
- Adding a new package schema handler does not require changes to the ingest or query layers.
- `Close` removes the temporary SQLite file.
- `make ci` passes.
