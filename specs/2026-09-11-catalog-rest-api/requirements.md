# Requirements

## Module And Ownership

- Implement the catalog server as a nested Go module at
  `examples/catalog_server`.
- Keep the HTTP handler, `/v1` wire models, OpenAPI contract, portable metadata
  model, metadata validation, namespaced property keys, property decoding, and
  executable server composition inside the nested module.
- Use module semantic versioning for the nested Go API and `/v1` only for the
  HTTP wire-contract version.
- Accept a `catalogv1.StoreReader` at the handler boundary so callers can mount
  the handler and set the outer catalog policy boundary.
- Keep server lifecycle, listen address, logging, and shutdown in the example
  application rather than the root library.
- Do not add `catalog/http` or another HTTP/OpenAPI package to the root module.
- Do not add `catalogv1.Package`, package metadata, bundle metadata, icon,
  metadata validation, or metadata property-key APIs to the core library.
- Do not mention examples in root `CLAUDE.md` or `specs/tech-stack.md`.
- Do not add a nested CI workflow, root Makefile target, workspace file, or other
  CI infrastructure for the nested module.

## Metadata Adaptation

- Define portable package and bundle presentation types in the nested module.
  They may include display name, descriptions, provider, maintainers, keywords,
  source repository, icon content and media type, bundle media type, and bundle
  release timestamp as required by the wire contract.
- Keep validation for URLs, email addresses, media types, timestamps, and other
  presentation values in the nested module.
- Define stable, namespaced graph and bundle property keys in the nested module.
- Decode portable metadata through the existing `UpdateGraph.Property` and
  `Bundle.Property` methods. Treat a missing optional property as absence and a
  malformed property as an error.
- Implement an `fbc.OLMPackageExtension` in the nested module. Use its per-blob
  callbacks and package accessor to collect legacy FBC metadata, and use its
  `PropertyWriter` during `FinalizePackage` to write package graph, icon, and
  bundle properties under the nested module's namespaced keys.
- Keep operator-registry and FBC source types behind the extension boundary;
  do not expose them in portable metadata or HTTP wire types.
- Preserve normal FBC import and SQLite behavior when the extension is not
  registered. Generic storage must not interpret the example's property keys.
- Preserve existing `FinalizePackage` partial-import behavior. A finalization
  error remains a per-package partial import error and does not introduce a new
  per-package rollback guarantee.
- Do not implement per-package atomic finalization or enforce uniqueness of
  bundle name/version/release identities.
- Do not change existing SQLite successor lookup behavior when separate catalogs
  reuse a bundle ID.

## Core Errors And Resolver

- Define `catalogv1.ErrNotFound` as the shared classification for lookup
  absence, with implementations wrapping it with operation and resource
  context.
- Make `StoreReader.Get` wrap `catalogv1.ErrNotFound` when a catalog is absent
  or hidden by the selected reader.
- Make `Catalog.GetPackage` wrap `catalogv1.ErrNotFound` when a package is
  absent.
- Make `CompositeUpdateGraph.GetGraph` wrap `catalogv1.ErrNotFound` when a child
  graph is absent.
- Do not use `ErrNotFound` for optional property absence, empty collections, or
  iterator completion.
- Make resolver package and graph lookup ignore only errors matching
  `catalogv1.ErrNotFound` and propagate every other read error with context.
- Evaluate package selection in complete descending-priority groups. Continue
  only if all catalogs in a group report absence, return a unique match, and do
  not read lower-priority package content after selection.
- Return a typed `resolverv1.AmbiguousPackageError` when more than one catalog
  at the selected priority contains the package. Include the package, priority,
  and deterministically sorted catalog names.
- Retain existing resolver sorting by name/version/release and optional
  deprecation preference. Revert and do not add bundle ID as an equal-identity
  tie-break.

## HTTP Contract

- Serve catalog, package, channel, bundle, icon, and recommendation resources
  beneath `/v1` as described in the design and checked-in OpenAPI contract.
- Allow cross-catalog requests to narrow the handler's base reader with standard
  Kubernetes label selector syntax. Omission selects every catalog visible
  through the base reader.
- Return every catalog-scoped package occurrence from package discovery. Sort by
  package name ascending, catalog priority descending, then catalog name
  ascending.
- Represent every nested child update graph as a channel. Use ordered segment
  arrays in JSON and one colon-delimited URL parameter; reject colons within a
  graph-name segment and impose no fixed nesting depth on the core model.
- Provide separate package-level and channel-level bundle collections. Invoke
  `ListBundles` on the addressed graph and preserve composite graph semantics.
- Do not accept channel paths as query parameters on bundle collections.
- Stream icon content decoded from the package graph property with its declared
  media type. Return not found when the icon property is absent.
- Accept recommendation requests at
  `POST /v1/recommendations/{package}`. Accept selector, cursor, and limit in the
  query and current bundle, channel paths, version constraint, and policy in the
  JSON body.
- Define `CatalogProvided` and `SelfCertified` policy values in the nested
  module, defaulting omission to `CatalogProvided`.
- Map catalog-provided upgrades to the existing `WithSuccessorsOf` resolver
  option and self-certified recommendations to its omission.
- Preserve resolver candidate order in recommendation responses. Do not add an
  HTTP rerank, multi-hop path computation, or bundle ID tie-break.
- Use stateless opaque cursors for collections, with a default limit of 50 and a
  maximum of 200.
- Bind cursors to normalized request inputs and a canonical hash of all selected
  catalog names, labels, digests, and priorities. Reject changed snapshots as
  stale and undecodable tokens as invalid.
- Return RFC 9457 `application/problem+json` responses with stable types for
  validation, unsupported policy, not found, ambiguity, invalid or stale cursor,
  method, and internal read or metadata errors.
- Complete reads needed for a success response before writing headers, except
  that an icon stream error after headers are sent terminates the response.
- Maintain a checked-in, spec-first OpenAPI YAML document in the nested module
  and test representative handler behavior against it.
- Do not add an HTTP framework, Kubernetes client, controller-runtime,
  operator-controller API, or catalog-v2 dependency.

## Acceptance Criteria

- The nested module builds and its tests pass independently while importing the
  root module as a normal consumer.
- A caller can construct the handler with a conforming `StoreReader`, and the
  executable can import FBC with the nested module's extension and mount the
  handler.
- Portable metadata and icons are written only under nested-module-owned
  namespaced properties and are decoded into the documented wire responses.
- Importing the same FBC without the extension retains normal core FBC and
  SQLite behavior.
- Finalization failures retain existing partial-import behavior; no test or
  contract promises per-package atomicity.
- Missing catalogs, packages, and child graphs satisfy
  `errors.Is(err, catalogv1.ErrNotFound)` while other read failures remain
  distinguishable and propagate through the resolver.
- Equal-priority matches produce a typed `AmbiguousPackageError`, and a unique
  higher-priority match prevents lower-priority package reads.
- Equal name/version/release candidates are not ordered by a newly introduced
  bundle ID tie-break, and no uniqueness enforcement is added.
- Package discovery preserves duplicate names as catalog-scoped results, nested
  channel paths round-trip, package and channel graph views remain distinct,
  and icon absence returns the documented not-found problem.
- Catalog-provided upgrades use immediate successors, self-certified
  recommendations ignore graph edges, and both preserve resolver order.
- Pagination, selector validation, Problem Details responses, and representative
  schemas match the OpenAPI contract.
- Root checks pass without root documentation or CI configuration referring to
  `examples/catalog_server`; nested-module checks are run directly rather than
  through new CI infrastructure.
