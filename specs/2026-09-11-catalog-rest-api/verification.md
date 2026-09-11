# Verification

## Implementation Correctness

- [ ] Confirm `catalogv1.Package` distinguishes package roots from nested
  `UpdateGraph` channels and all catalog implementations return it.
- [ ] Confirm package and bundle metadata methods return typed values and errors,
  while identity and existing `Deprecated` behavior remain authoritative.
- [ ] Confirm the icon result provides streamable content and media type without
  closing the type against a future icon-location representation, and the
  handler streams content or reports absence correctly.
- [ ] Confirm FBC import populates every available portable field without
  exposing operator-registry types from the core or HTTP APIs.
- [ ] Confirm package absence has a stable typed outcome and recommendation
  evaluates catalogs one complete descending-priority group at a time.
- [ ] Confirm any read error in the current priority group fails recommendation,
  multiple matches in that group are ambiguous, and exactly one match prevents
  package reads from all lower-priority groups.
- [ ] Confirm `CatalogProvided` maps to `WithSuccessorsOf` and queries immediate
  successors, while `SelfCertified` omits that option and retains all other
  constraints.
- [ ] Confirm graph implementations determine successor eligibility, the
  resolver determines current candidate ordering, and that order reaches
  recommendation responses unchanged.
- [ ] Confirm the spec does not claim the current graph/resolver interfaces can
  express weighted or multi-hop ranking, while the REST response leaves room
  for such a future core API change.
- [ ] Confirm catalog selectors only narrow the handler's base StoreReader and
  invalid selectors produce the documented validation problem.
- [ ] Confirm package discovery and recommendation use the same catalog selector
  query parameter and parser.
- [ ] Confirm recommendation accepts catalog selector, cursor, and limit only in
  the query and accepts resolution criteria only in the JSON body.
- [ ] Confirm package occurrences are catalog-scoped and sorted by package name,
  catalog priority descending, and catalog name.
- [ ] Confirm arbitrary nested channels round-trip through path-array responses
  and colon-delimited channel resource URLs, with colons rejected inside graph
  name segments and no fixed nesting depth imposed on the domain model.
- [ ] Confirm package and channel bundle collections call `ListBundles` on the
  addressed graph, composite graphs include descendant bundles, and bundle
  collection endpoints accept no channel-path query parameter.
- [ ] Confirm every collection uses deterministic ordering, a default limit of
  50, a maximum of 200, and a stateless opaque cursor.
- [ ] Confirm cursors contain hashes rather than complete request or catalog
  snapshots, hash every selected catalog's name, labels, digest, and priority in
  canonical order, and report stale-cursor conflicts when those inputs change.
- [ ] Confirm cursor encoding requires no version field, an undecodable cursor
  produces the documented invalid-cursor problem, and clients can restart from
  the first page.
- [ ] Confirm discovery operations and the current recommendation priority group
  fail before success headers when a required catalog read fails.
- [ ] Confirm all implemented routes, methods, request fields, response fields,
  icon outcomes, and Problem Details responses match the checked-in OpenAPI
  document.
- [ ] Confirm resolver tests, rather than HTTP tests, exhaustively cover catalog
  priority groups, package absence versus read errors, ambiguity, lower-priority
  read avoidance, filtering, successor delegation, deduplication, ordering, and
  deprecation preference.
- [ ] Confirm handler tests use a fake `catalogv1.StoreReader` with no SQLite or
  FBC fixtures and cover only handler-owned transport and orchestration behavior,
  with representative recommendation cases for input translation, output-order
  preservation, and error mapping.
- [ ] Confirm shared catalog-domain fakes in `internal/util/test` contain no HTTP
  concepts or package-specific assertion DSL, while HTTP-specific helpers remain
  local to `catalog/http` tests.
- [ ] Confirm the example opens a store and mounts the handler while production
  library packages do not start an HTTP server.
- [ ] Confirm the Go API lives in `catalog/http` while `/v1` versions only the
  HTTP wire contract.
- [ ] Run `make test` and confirm new code has at least 70 percent statement
  coverage and overall project coverage does not decrease.
- [ ] Run `make ci` successfully.

## Project Conventions

- [ ] Check `specs/conventions.md`: public API changes are tested, commit and PR
  metadata follow project conventions, and no lint suppression is introduced.
- [ ] Check `specs/mission.md`: the result is an embeddable library, uses inert
  typed domain values and standalone projection functions where practical,
  keeps implementation details internal, and adds no cluster behavior.
- [ ] Check `specs/tech-stack.md`: implementation uses Go 1.25.7, standard
  `net/http`, existing catalog/resolver/SQLite abstractions, and existing
  Kubernetes label parsing without adding an HTTP framework.
- [ ] Confirm legacy operator-framework dependencies remain confined to FBC
  adaptation and no legacy type appears in a public REST or metadata type.
- [ ] Confirm catalog-v2 ingestion, extended metadata query/filter APIs,
  referenced-icon behavior, multi-hop paths, and OpenAPI code generation remain
  deferred.
- [ ] Confirm exported names and comments follow Go conventions and conversion
  helpers use the project's `FromX`/`ToY` naming convention where applicable.
