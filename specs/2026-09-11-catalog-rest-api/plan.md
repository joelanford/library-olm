# Implementation Plan

1. Establish `examples/catalog_server` as a nested Go module that depends on the
   root module. Keep application startup and shutdown there, and do not add root
   documentation, root build integration, a workspace file, or nested CI
   infrastructure.
2. Retain only the generic core property boundary needed by consumers. Ensure
   bundles and update graphs expose raw JSON properties, without adding
   `catalogv1.Package`, package metadata, bundle metadata, icon, validation, or
   application property-key APIs.
3. Make lookup absence consistent. Add `catalogv1.ErrNotFound` and update
   `StoreReader.Get`, `Catalog.GetPackage`, and
   `CompositeUpdateGraph.GetGraph` implementations and tests so absence wraps
   it while optional-property absence and other failures do not.
4. Correct resolver error handling. Evaluate complete descending-priority
   groups, ignore only `ErrNotFound`, propagate other package and graph read
   errors, stop after a unique match, and return typed
   `AmbiguousPackageError` values with sorted catalog names. Keep existing
   candidate sorting and remove the proposed bundle ID tie-break.
5. Define the portable presentation model inside the nested module. Add package,
   icon, and bundle metadata types; validation for their URL, email, media type,
   and timestamp fields; namespaced graph and bundle property keys; and focused
   property encoding/decoding tests.
6. Implement the nested module's `fbc.OLMPackageExtension`. Capture required
   legacy FBC values in callbacks, select and validate portable metadata during
   `FinalizePackage`, and write package graph, icon, and bundle properties with
   the namespaced keys. Verify import without the extension is unchanged and
   preserve existing partial-import semantics. Do not add per-package
   transactions or name/version/release uniqueness enforcement.
7. Move or implement the spec-first OpenAPI contract and HTTP package in the
   nested module. Define `/v1` catalog, package, nested channel, package/channel
   bundle, icon, and recommendation resources plus RFC 9457 Problem Details.
8. Implement shared HTTP mechanics in the nested module: strict path, query, and
   body validation; selectors that narrow the supplied reader; domain-to-wire
   projection; deterministic collection ordering where specified; compact
   stateless cursors bound to normalized requests and canonical catalog
   snapshots; and pre-header read completion.
9. Implement browsing and recommendation behavior. Decode portable metadata
   only through generic properties, preserve package and channel graph views,
   stream icons, map `CatalogProvided` and `SelfCertified` to existing resolver
   options, preserve resolver candidate order, and map typed absence,
   ambiguity, and read/decode failures to documented problems.
10. Add focused tests in the owning module. Root tests cover generic properties,
    not-found outcomes, unchanged FBC/SQLite behavior, extension hooks, and
    resolver selection/errors. Nested tests cover metadata adaptation and
    validation, handler transport/orchestration, cursor behavior, icon
    streaming, and representative OpenAPI conformance without duplicating the
    root store or resolver matrices.
11. Run root formatting, tidy, tests, lint, generated-file verification, build,
    and `make ci`. Separately run formatting, tidy verification, tests, and build
    from `examples/catalog_server`; do not add automation that folds the nested
    module into root CI.
