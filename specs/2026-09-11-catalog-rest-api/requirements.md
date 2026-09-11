# Requirements

- Provide a public `catalog/http` package whose constructor accepts a
  `catalogv1.StoreReader` and returns an `http.Handler` without starting or
  configuring an HTTP server.
- Use module semantic versioning for the Go API and retain `/v1` only as the
  independent HTTP wire-contract version.
- Serve catalog, package, channel, bundle, icon, and recommendation resources
  beneath `/v1` as defined in the design and checked-in OpenAPI contract.
- Allow cross-catalog requests to narrow the handler's base reader with the
  standard Kubernetes label selector syntax. Omission selects every catalog
  visible through the base reader.
- Return every catalog-scoped occurrence from package discovery. Sort package
  occurrences by package name ascending, catalog priority descending, then
  catalog name ascending.
- Represent every nested child update graph as a channel and identify it with
  an ordered path of graph-name segments. Encode resource URL paths as one
  colon-delimited path parameter while retaining segment arrays in JSON and the
  domain model; reject colons within individual graph-name segments.
- Provide separate package-level and channel-level bundle collection endpoints.
  Package-level listing uses the package root graph; channel-level listing uses
  the addressed graph. Composite graph listings include bundles from all
  descendants according to `UpdateGraph.ListBundles`.
- Do not accept channel paths on bundle collection endpoints. Clients querying
  several specific channels make separate requests, each with its own cursor
  and limit.
- Add a `catalogv1.Package` interface for package-root graph behavior, typed
  package metadata, and lazy icon access. Change catalog package queries to
  return this interface.
- Add typed bundle metadata for optional media type and release timestamp while
  retaining existing bundle identity and URI methods.
- Preserve `catalogv1.Deprecated` as the format-neutral source of package,
  channel, and bundle deprecation messages.
- Define an extensible concrete icon result type whose initial fields provide
  streamable content and its media type. Do not make the result a closed source
  enumeration or otherwise prevent adding an icon location later.
- Stream available icon content with its declared media type and return not
  found when no icon exists.
- Populate all portable metadata available from legacy FBC during import, and
  make it available through the typed core interfaces.
- Do not expose FBC types, raw catalog properties, vendor-specific attributes,
  or catalog-v2 types in the core metadata or HTTP wire models.
- Accept recommendation requests at
  `POST /v1/recommendations/{package}`. Accept catalog selector, cursor, and
  limit as query parameters. Accept optional current bundle identity, channel
  paths, semantic-version constraint, and upgrade constraint policy in the JSON
  body.
- Use the same catalog selector query parameter and Kubernetes selector syntax
  for `GET /v1/packages` and recommendation requests.
- Require a client continuing a recommendation collection to repeat the same
  JSON body, and bind its cursor to that body and the catalog selector.
- Define local HTTP `CatalogProvided` and `SelfCertified` upgrade constraint
  policy values, defaulting omitted policy to `CatalogProvided`.
- For `CatalogProvided` upgrades, return only immediate candidates supplied by
  the selected update graphs by supplying the resolver's existing
  `WithSuccessorsOf` option. For `SelfCertified`, omit that option to ignore
  graph edges and rank all bundles satisfying the remaining request constraints.
- Preserve the canonical resolver's candidate order in the response and do not
  rerank or construct multi-hop paths in the HTTP package. Keep the ordered-list
  response compatible with a future core graph/resolver API change for
  format-aware edge weighting or shortest-path recommendation behavior.
- Prefer non-deprecated recommendation candidates while retaining deprecated
  candidates after them.
- Evaluate recommendation catalogs in descending priority groups. Within each
  group, check every catalog for the package, fail on any read error, return an
  ambiguity problem for multiple matches, select exactly one match, or continue
  when there are no matches.
- Stop probing package content after selecting a package occurrence; unreadable
  catalogs in lower-priority groups must not affect that recommendation.
- Distinguish package absence from catalog read failures. Discovery operations
  that aggregate selected catalogs fail when a catalog required for that result
  is unreadable.
- Use stateless opaque cursors for all collections, with a default limit of 50
  and maximum limit of 200.
- Keep cursors compact by storing hashes of normalized request inputs and the
  selected catalog snapshot rather than embedding the full inputs or snapshot.
  Build the catalog snapshot hash from a canonical catalog-name-sorted sequence
  containing every selected catalog's name, labels, digest, and priority, with
  label keys sorted. Reject changed snapshots as stale rather than silently
  continuing.
- Treat cursor encoding as an implementation detail with no required version
  field or compatibility guarantee across handler upgrades. Reject an
  undecodable cursor and require the client to restart from the first page.
- Return errors as RFC 9457 `application/problem+json` documents with stable
  problem types and appropriate HTTP status codes.
- Maintain a checked-in, spec-first OpenAPI YAML document and tests that verify
  handler behavior and representative schemas against it.
- Test each behavior exhaustively in its owning package. Use fake
  `catalogv1.StoreReader` and domain values for handler tests, without SQLite or
  FBC fixtures, and do not duplicate catalog-store or resolver behavior matrices
  through the HTTP layer.
- Keep catalog-domain test fakes shared by resolver and HTTP tests in
  `internal/util/test`; keep HTTP request, response, Problem Details, OpenAPI,
  and stream helpers local to `catalog/http` tests.
- Replace `examples/catalog_server` with a working example that composes the
  SQLite store and public handler while keeping server lifecycle in example
  application code.
- Do not add a CLI, Kubernetes client, controller-runtime dependency,
  operator-controller API dependency, or HTTP framework dependency.

## Acceptance Criteria

- A consumer can construct and mount the handler with any conforming
  `catalogv1.StoreReader`.
- Catalog and package collection requests honor valid label selectors and
  reject invalid selectors with a typed validation problem.
- Package discovery returns duplicate names as separate catalog-scoped items in
  the specified deterministic order.
- Package detail exposes the portable FBC presentation fields, channel
  responses preserve arbitrary nesting as path arrays, and bundle responses
  expose identity plus available release metadata.
- Package and channel bundle endpoints return their respective graph views, and
  a composite graph includes the union of bundles from its descendants.
- Fake package icon content is streamed with its declared content type, and
  absent icons return a not-found problem.
- Install recommendation without a current bundle returns resolver-ranked
  candidates from the selected catalog and requested channels and versions.
- Catalog-provided upgrade recommendation returns immediate successors only;
  self-certified recommendation ignores update edges while preserving all
  other filters.
- Equal-priority package matches produce an ambiguity problem, and a catalog
  selector can narrow the request to a lower-priority catalog.
- A read error in the priority group currently checked by recommendation fails
  the request, while lower-priority package reads are not attempted after a
  higher-priority match is selected.
- A missing package yields not found, while an unreadable catalog required by a
  discovery operation fails that operation.
- Collection continuation returns the next deterministic page without
  server-side session state, and catalog replacement causes the old cursor to
  return a stale-cursor conflict.
- Malformed JSON, invalid limits, invalid channel paths, invalid semantic
  constraints, unsupported policies, missing resources, invalid or stale cursors,
  ambiguity, and internal failures have documented Problem Details responses.
- The checked-in OpenAPI document describes every endpoint, parameter, request,
  success response, redirect, content type, and error response implemented by
  the handler.
- The example builds and demonstrates mounting the handler, but no production
  package calls `http.ListenAndServe`.
- New and changed code has tests, new code reaches at least 70 percent statement
  coverage, overall coverage does not decrease, and `make ci` passes.
