---
status: in-progress
---
# Catalog REST API

## Summary

Build a versioned catalog REST API and checked-in OpenAPI contract in the
`examples/catalog_server` nested Go module. The example module is a complete
consumer of the root library: it owns HTTP transport, wire models, portable
presentation metadata, metadata validation and decoding, namespaced property
keys, FBC metadata adaptation, and server lifecycle.

The root library remains transport- and presentation-neutral. It does not gain
a `Package` abstraction, package or bundle metadata types, icon APIs, HTTP
packages, or OpenAPI artifacts. The server reads example-owned metadata through
the existing `UpdateGraph.Property` and `Bundle.Property` APIs.

The only root-library behavior changes retained by this work are consistent
not-found classification and resolver error handling. `catalogv1.ErrNotFound`
wraps absence from `StoreReader.Get`, `Catalog.GetPackage`, and
`CompositeUpdateGraph.GetGraph`. The resolver ignores only that classified
absence, propagates other read errors, and reports equal-priority package
matches with a typed `AmbiguousPackageError`. Existing resolver ordering is not
changed with a bundle ID tie-break.

## Design

### Module boundary

`examples/catalog_server` is an independently buildable nested Go module. It
depends on the root `library-olm` module as a consumer and may use dependencies
needed by its API and example application without adding them to the root
module's public surface.

The nested module owns:

- the `net/http` handler and all routing, request parsing, response projection,
  pagination, cursor handling, and RFC 9457 Problem Details mapping
- the spec-first OpenAPI document and contract tests
- portable package and bundle presentation metadata types
- metadata validation, including URL, email, media type, and timestamp rules
- namespaced graph and bundle property-key constants
- functions that decode those properties from `catalogv1.UpdateGraph` and
  `bundlev1.Bundle`
- an implementation of `fbc.OLMPackageExtension` that extracts legacy FBC
  presentation data and writes the example-owned namespaced properties
- executable composition, including opening/importing a SQLite store, mounting
  the handler, listening, logging, and graceful shutdown

The root module continues to own generic bundle, catalog, FBC import, SQLite,
and resolver behavior. In particular, the root module does not define the
portable metadata schema merely because one example consumes it. Root
`CLAUDE.md` and `specs/tech-stack.md` remain descriptions of the root library
and do not mention the example module.

The nested module gets no dedicated CI workflow, Makefile integration, or
other nested CI infrastructure. It is verified directly with Go commands in
its own module in addition to normal root-module checks.

### Metadata adaptation

Portable metadata is an application concern in this design. The nested module
defines the types needed by its wire contract, including package presentation
fields, icon content and media type, and optional bundle media type and release
timestamp. It also owns conversion from legacy FBC source data and validation
of those values.

The example registers its `fbc.OLMPackageExtension` with the FBC importer.
Per-blob callbacks retain the source data needed for finalization. During
`FinalizePackage`, the extension selects and validates the portable values and
writes JSON values under namespaced keys:

- package metadata and icon data are graph properties on the package root
- bundle metadata is a bundle property

The handler later decodes these properties through the existing generic
property APIs. Missing optional properties remain absence, not
`catalogv1.ErrNotFound`. Invalid stored values are surfaced as read/decode
errors and mapped to internal Problem Details responses rather than silently
treated as absent.

No default FBC behavior changes. Import without the example extension produces
the same normalized catalog as before, and generic SQLite storage remains
unaware of the property schemas. Existing `FinalizePackage` partial-import
semantics also remain unchanged: extension finalization errors are collected as
per-package partial import errors, and writes already made through the shared
writer are not rolled back on a per-package boundary.

Per-package atomic finalization and enforcement of unique
name/version/release identities are explicitly out of scope. The work does not
change the resolver to use bundle ID as a tie-break when two bundles compare
equally by name/version/release.

### Core read outcomes

`catalogv1.ErrNotFound` is the common absence classification for these lookup
operations:

- `StoreReader.Get` when the named catalog does not exist or is not visible
  through the selected reader
- `Catalog.GetPackage` when the catalog has no package with that name
- `CompositeUpdateGraph.GetGraph` when the child graph does not exist

Implementations add context while wrapping `ErrNotFound`, so callers use
`errors.Is`. Iterator exhaustion, optional property absence, and empty
collections are not represented by this error.

Resolver package selection evaluates catalogs in descending priority groups.
It examines every catalog in the current group, ignores only errors wrapping
`catalogv1.ErrNotFound`, propagates every other read error, returns a typed
`resolverv1.AmbiguousPackageError` for multiple matches, and selects a unique
match. It proceeds to a lower-priority group only if every catalog in the
current group reports package absence. Once a unique package is selected, it
does not read package content from lower-priority groups.

Graph-path selection follows the same rule: a missing graph means the requested
path has no match, while another `GetGraph` error is propagated. Resolver
candidate sorting otherwise retains its existing version/release and optional
deprecation behavior. No bundle ID tie-break is introduced for equal
name/version/release values.

### Resource identity and terminology

The wire API uses the terms **package**, **channel**, and **bundle**, but these
are wire concepts and do not require a new core `catalogv1.Package` type.

- A package occurrence is scoped by catalog name and package graph name.
  Discovery does not merge duplicate package names across catalogs.
- Every child update graph, including a child of another child, is represented
  as a channel.
- A channel is identified by an ordered path of graph-name segments relative to
  its package root. JSON uses a string array. URLs encode the path as one
  colon-delimited parameter beneath `/channels/`; a colon is therefore invalid
  within an individual graph-name segment.
- A bundle is identified within its catalog by bundle ID and reports its
  existing name/version/release identity and URI. Portable presentation fields
  come from example-owned properties.

Package discovery is sorted by package name ascending, catalog priority
descending for equal package names, and catalog name ascending for equal names
and priorities. Other collection sort keys are documented in OpenAPI. The
recommendation response preserves resolver order; it does not add a bundle ID
tie-break for equal name/version/release values.

### HTTP resources

The handler serves these resource families beneath `/v1`:

- `GET /v1/catalogs` lists selected catalog metadata.
- `GET /v1/catalogs/{catalog}` returns one catalog's metadata.
- `GET /v1/packages` lists catalog-scoped package occurrences across selected
  catalogs.
- `GET /v1/catalogs/{catalog}/packages/{package}` returns package detail.
- `GET /v1/catalogs/{catalog}/packages/{package}/channels` lists all descendant
  channels with path arrays.
- `GET /v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}` returns
  one nested channel.
- `GET /v1/catalogs/{catalog}/packages/{package}/bundles` lists bundles visible
  from the package root graph.
- `GET /v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles`
  lists bundles visible from the addressed channel graph.
- `GET /v1/catalogs/{catalog}/packages/{package}/bundles/{bundleID}` returns one
  bundle visible from the package.
- `GET /v1/catalogs/{catalog}/packages/{package}/icon` streams the icon stored in
  the package graph property or returns not found.
- `POST /v1/recommendations/{package}` returns ranked install or immediate
  upgrade candidates.

Cross-catalog requests accept a Kubernetes label selector that can only narrow
the `catalogv1.StoreReader` supplied to the handler. Collection endpoints accept
cursor and limit parameters. Package-level and channel-level bundle collections
remain separate; bundle collections do not accept channel paths as query
parameters. Composite graph listing follows `UpdateGraph.ListBundles` and
therefore includes descendant bundles according to core graph semantics.

Wire types never expose Go interface values, operator-registry types, raw FBC
objects, or proposed catalog-v2 types. They may expose the portable fields
decoded from the example-owned properties.

### Recommendation semantics

The recommendation request uses the package path for the package name, query
parameters for catalog selector, cursor, and limit, and a JSON body for:

- optional current bundle identity, where omission means initial installation
- optional channel paths, where omission means all channels
- optional semantic-version constraint, where omission means all versions
- optional `upgradeConstraintPolicy`, with `CatalogProvided` as the default and
  `SelfCertified` as the other initial value

The policy values are local to the nested module. `CatalogProvided` with a
current bundle maps to the resolver's existing `WithSuccessorsOf` option.
`SelfCertified` omits that option and considers all bundles satisfying the
remaining constraints. Both may use the existing resolver option that prefers
non-deprecated bundles. The HTTP layer preserves resolver candidate order and
does not compute multi-hop upgrade paths or rerank equal identities.

The recommendation cursor is bound to both the normalized query and JSON body.
A client repeats the same body when continuing a result set.

### Pagination and errors

Collection endpoints use compact, stateless opaque cursors. The default limit
is 50 and the maximum is 200. A cursor binds the normalized request inputs and a
canonical snapshot of selected catalog names, labels, digests, and priorities.
It stores hashes rather than complete request or catalog data. A changed catalog
snapshot produces a stale-cursor conflict; an undecodable cursor produces an
invalid-cursor response.

Errors use `application/problem+json` following RFC 9457. Stable problem types
cover malformed input, invalid selectors or constraints, unsupported policy,
not found, invalid or stale cursor, equal-priority ambiguity, method not
allowed, and internal catalog or metadata failures. Domain reads required for a
success response complete before success headers are written, except that a
stream failure after icon headers are sent can only terminate the response.

The checked-in OpenAPI YAML in the nested module is the authoritative wire
contract. Contract tests validate representative requests, responses, content
types, status codes, and schemas. OpenAPI-driven Go generation is deferred.

## Out Of Scope

- adding `catalogv1.Package` or changing catalog package methods to return one
- adding core package metadata, bundle metadata, icon, validation, or property
  key APIs
- changing normal FBC import or SQLite query/storage behavior
- changing `FinalizePackage` partial-import semantics or making finalization
  atomic per package
- enforcing name/version/release uniqueness
- correcting SQLite successor lookup when separate catalogs reuse a bundle ID
- adding a resolver bundle ID tie-break
- importing or adapting `olm.package.v2`
- metadata querying, filtering, facets, or exposing arbitrary properties
- referenced-icon redirect or proxy behavior
- multi-hop or weighted upgrade path computation
- OpenAPI code generation
- authentication, authorization, or Kubernetes clients
- adding nested-module CI, root Makefile integration, or root documentation for
  the example module
