# Implementation Plan

1. Extend the core domain model. Add `catalogv1.Package`, typed package and
   bundle metadata values, validated URL/email/media-time representations as
   needed, and an extensible icon result containing streamable content and media
   type. Change catalog package
   methods to return `Package`, update bundle metadata access, retain the
   existing deprecation capability, and update in-repository implementations
   and test doubles.
2. Make read outcomes explicit. Add a typed or sentinel catalog not-found error,
   update SQLite queries to return it consistently, and change resolver catalog
   selection to evaluate complete priority groups, ignore only package absence,
   propagate read failures from the group being evaluated, and stop before
   reading lower-priority package content after a unique match.
3. Populate portable FBC metadata. Extend normal FBC import/finalization to
   validate and retain package presentation data, icons, and available bundle
   release metadata in the SQLite-backed model without exposing legacy
   operator-registry types through public domain APIs.
4. Validate canonical recommendation behavior. Keep the policy enum in the HTTP
   API, map `CatalogProvided` to the existing `WithSuccessorsOf` resolver option,
   and map `SelfCertified` to its omission. Preserve graph-defined immediate
   successor eligibility and resolver-defined candidate ordering, and add tests
   for channel, version, deprecation, priority, ambiguity, absence, and
   read-error interactions.
5. Define the HTTP contract. Add the spec-first OpenAPI YAML under
   `catalog/http`, including all resource schemas, recursive channel-path
   representation, selectors, recommendation inputs, cursor fields, icon
   content responses, and RFC 9457 problem types.
6. Implement shared HTTP mechanics. Add handler construction and routing,
   strict request decoding and validation, Problem Details rendering,
   deterministic sorting, and compact stateless cursor encoding/validation
   bound to hashes of normalized request inputs and canonical selected-catalog
   name, label, digest, and priority snapshots.
7. Implement catalog and package discovery. Add catalog list/detail and
   cross-catalog package listing with selector handling, all-or-nothing read
   errors for catalogs required by the result, portable summary projection,
   specified ordering, and pagination.
8. Implement catalog-scoped browsing. Add package detail, recursive channel
   list/detail, separate package-level and channel-level bundle collections,
   bundle detail, and package icon operations with deterministic collection
   ordering and pagination.
9. Implement the separate multi-catalog recommendation resource. Decode and
   validate the package path plus catalog selector, cursor, and limit query
   parameters separately from the JSON recommendation criteria, select the
   request-scoped reader, map inputs to canonical resolver options, preserve
   resolver candidate order, paginate the result, and map absence, ambiguity,
   stale snapshots, and read failures to documented problems.
10. Replace the catalog server prototype with a minimal compiling example that
    opens a SQLite store and mounts the public handler in caller-owned server
    code.
11. Add small shared catalog-domain fakes under `internal/util/test`. Use them to
    unit test resolver-owned priority-group, absence, ambiguity, read-error, and
    lower-priority read-boundary behavior, and to unit test the handler through
    `httptest` without SQLite or FBC fixtures. Keep the handler suite focused on
    transport and orchestration: decoding and validation, catalog/resolver input
    translation, projection, handler-defined ordering, pagination and cursor
    invalidation after fake catalog metadata changes, resolver-order
    preservation, Problem Details, content types, icon streaming, and the
    pre-header read boundary. Verify representative
    handler requests and responses against the OpenAPI schemas without repeating
    the resolver or store behavior matrices.
12. Run formatting, dependency tidying if needed, generated-file verification,
    tests, lint, build, and the complete `make ci` check. Confirm no catalog-v2,
    query/filter, server framework, cluster, or operator-controller dependency
    entered the implementation.
