# Verification

The unchecked items below define the verification to perform. They do not claim
that the implementation has been verified.

## Architecture And Scope

- [x] Confirm `examples/catalog_server` has its own `go.mod` and consumes the
  root module without moving example-specific dependencies or APIs into it.
- [x] Confirm the nested module owns HTTP routing, wire types, OpenAPI,
  presentation metadata, validation, property keys, property decoding, FBC
  adaptation, and executable server lifecycle.
- [x] Confirm the root module has no `catalog/http` package and no
  `catalogv1.Package`, package metadata, bundle metadata, icon, portable
  metadata validation, or application property-key API.
- [x] Confirm root `CLAUDE.md` and `specs/tech-stack.md` do not mention examples.
- [x] Confirm no nested CI workflow, root Makefile integration, Go workspace, or
  other CI infrastructure was added for `examples/catalog_server`.

## Metadata And Import

- [x] Confirm portable package, icon, and bundle metadata types and validation
  live only in the nested module.
- [x] Confirm all metadata keys are namespaced constants owned by the nested
  module and the handler reads values through `UpdateGraph.Property` and
  `Bundle.Property`.
- [x] Confirm missing optional properties decode as absence while malformed
  properties return errors and become internal Problem Details responses.
- [x] Confirm the nested module's `fbc.OLMPackageExtension` captures legacy FBC
  data and writes package metadata and icon properties to the package root graph
  and bundle metadata to bundle properties.
- [x] Confirm operator-registry types do not appear in portable metadata or HTTP
  wire APIs.
- [x] Confirm importing without the extension preserves normal FBC normalization
  and SQLite behavior and does not write the example's namespaced properties.
- [x] Confirm `FinalizePackage` errors retain existing partial-import behavior,
  including the absence of a per-package rollback guarantee.
- [x] Confirm no per-package atomicity mechanism or name/version/release
  uniqueness enforcement was added.

## Core Errors And Resolver

- [x] Confirm missing `StoreReader.Get`, `Catalog.GetPackage`, and
  `CompositeUpdateGraph.GetGraph` results each wrap `catalogv1.ErrNotFound` and
  retain useful context.
- [x] Confirm optional-property absence, empty iteration, and non-absence read
  failures do not match `catalogv1.ErrNotFound`.
- [x] Confirm resolver package selection checks complete descending-priority
  groups, ignores only `ErrNotFound`, and propagates every other read error.
- [x] Confirm a unique package match prevents package reads from lower-priority
  groups.
- [x] Confirm equal-priority matches return a typed
  `resolverv1.AmbiguousPackageError` containing package, priority, and sorted
  catalog names.
- [x] Confirm graph-path lookup treats only `ErrNotFound` as no match and
  propagates other `GetGraph` errors.
- [x] Confirm resolver ordering retains existing name/version/release and
  deprecation behavior and has no bundle ID tie-break for equal identities.

## HTTP Contract

- [x] Confirm the handler accepts a `catalogv1.StoreReader`, selectors only
  narrow that reader, and invalid selectors produce the documented validation
  problem.
- [x] Confirm package discovery returns catalog-scoped duplicates sorted by
  package name, catalog priority descending, and catalog name.
- [x] Confirm arbitrary nested channels round-trip as path arrays and
  colon-delimited URL values, colons are rejected within segments, and no fixed
  nesting depth is imposed on the core model.
- [x] Confirm package and channel bundle endpoints call `ListBundles` on the
  addressed graph and do not accept channel-path query parameters.
- [x] Confirm package and bundle responses contain portable metadata decoded
  from namespaced properties without exposing raw properties or FBC types.
- [x] Confirm icon content is streamed with its declared media type, absent icon
  properties produce not found, and post-header stream failures terminate the
  response without attempting a JSON replacement body.
- [x] Confirm recommendation query parameters contain selector, cursor, and
  limit while the JSON body contains current bundle, channel paths, version
  constraint, and policy.
- [x] Confirm `CatalogProvided` maps to `WithSuccessorsOf`, `SelfCertified`
  omits it, and recommendation responses preserve resolver order without an HTTP
  bundle ID tie-break or multi-hop computation.
- [x] Confirm collections use the documented ordering, default limit of 50,
  maximum limit of 200, and stateless opaque cursors.
- [x] Confirm cursor hashes bind normalized request inputs and a canonical
  selected-catalog snapshot of names, labels, digests, and priorities; changed
  snapshots are stale and undecodable tokens are invalid.
- [x] Confirm required reads complete before success headers except for errors
  encountered while streaming an already-started icon response.
- [x] Confirm all routes, methods, parameters, schemas, content types, success
  responses, and Problem Details outcomes match the nested module's checked-in
  OpenAPI document.

## Tests And Conventions

- [x] Confirm root tests exhaustively cover generic property behavior,
  not-found wrapping, FBC extension plumbing and unchanged no-extension
  behavior, SQLite behavior, and resolver priority/error semantics.
- [x] Confirm nested-module tests cover metadata extraction, validation and
  decoding, transport and orchestration, cursors, icon streaming, and
  representative OpenAPI conformance without duplicating root behavior matrices.
- [x] Run root `make test` and confirm changed root code meets project coverage
  requirements without reducing overall coverage.
- [x] Run root `make ci` successfully.
- [x] From `examples/catalog_server`, run `gofmt` checks, `go mod tidy` with no
  diff, `go test ./...`, and `go build ./...` directly.
- [x] Check `specs/conventions.md`: public API changes are tested, commit and PR
  metadata follow conventions, and no lint suppression is introduced.
- [x] Check `specs/mission.md`: the root remains a cluster-independent catalog
  library and application-specific transport and metadata stay at the consumer
  boundary.
- [x] Confirm deferred work remains absent: core presentation APIs, normal
  FBC/SQLite behavior changes, per-package atomicity, identity uniqueness
  enforcement, cross-catalog duplicate-bundle-ID successor lookup changes,
  catalog-v2 ingestion, metadata filtering, referenced-icon behavior, multi-hop
  paths, OpenAPI generation, and nested CI infrastructure.
