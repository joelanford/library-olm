---
status: idea
---
# Catalog REST API

Provide an embeddable, format-neutral REST API and OpenAPI contract over `catalogv1.StoreReader` for discovering Kubernetes software update graphs, inspecting the bundles in a selected catalog and graph, and recommending install or upgrade candidates. The API must project both legacy FBC (`olm.package`, `olm.channel`, `olm.bundle`) and the proposed `olm.package.v2` catalog format into a common domain without becoming a standalone server or taking on cluster lifecycle responsibilities.

## Intended Outcome

Consumers can mount a library-provided `http.Handler` in their own application
and query a collection of catalogs through a stable HTTP and OpenAPI contract.
The handler operates over a caller-provided `catalogv1.StoreReader`, including
its catalog selection, and delegates recommendation semantics to the canonical
resolver rather than reimplementing them in the transport layer.

The API supports three related operations:

1. List and query update graphs across all selected catalogs, with
   summary-level metadata suitable for discovery experiences.
2. List and query bundles within a specified catalog and update graph, with
   summary-level metadata suitable for choosing a version.
3. Resolve recommended install or upgrade candidates. Installation is the
   special case where no current bundle is supplied; upgrade starts from a
   supplied bundle identity and returns its applicable successors.
4. Fetch the icon for a specific package through a dedicated endpoint, returning
   the icon bytes with their declared media type rather than embedding large
   binary data in discovery responses.

## Format Support

The common API must support both catalog representations:

1. Legacy FBC documents using `olm.package`, `olm.channel`, `olm.bundle`, and
   `olm.deprecations`. Package, channel, bundle, and deprecation data may need
   to be extracted through the FBC importer extension mechanism and retained as
   catalog properties.
2. The proposed catalog-v2 format represented by
   `~/projects/work/joelanford/catalog-overhaul/internal/catalog/model.go`,
   including `olm.package.v2`, its version streams and bundles, and its
   package-level and bundle-level `query` maps.

The API must not expose either source format directly as its wire model. It
should instead build on a shared catalog-domain projection so future formats can
provide the same discovery, querying, and recommendation behavior.

## Scope

- Define format-neutral summary and detail types for update graphs and bundles.
- Define an adapter or importer strategy that projects FBC and catalog-v2 data
  into those types while preserving required update relationships.
- Provide a caller-mountable HTTP handler, with no listen address, server
  lifecycle, authentication policy, or application configuration surface.
- Publish and test an OpenAPI contract that describes endpoints, request and
  response types, icon media responses, pagination, validation failures,
  missing resources, and resolution ambiguity.
- Reuse `catalogv1.StoreReader`, `catalogv1.Catalog`, `catalogv1.UpdateGraph`,
  `catalogv1.CompositeUpdateGraph`, and `resolverv1.Resolve` where their
  existing semantics apply.
- Retire or replace the incomplete `examples/catalog_server` prototype only
  when the library API supersedes its intended use.

## Open Design Questions

- What terms should the wire API use for the root software unit and its nested
  paths: package and update graph, package and channel, or more generic terms?
- Which fields belong in the portable summary model? Candidate common fields
  include identity, display name, descriptions, provider, maintainers,
  keywords, icon, version and release, image reference, media type, release
  date, and deprecation. Lifecycle and platform compatibility may or may not
  be common enough for the initial contract.
- How should vendor-specific metadata be represented without hardwiring every
  vendor schema into the API? The v2 `query` maps demonstrate scalar, list, and
  structured values, while FBC metadata can be arbitrary JSON properties.
- How should clients filter and discover those extended attributes? Options
  include a typed filter grammar over a namespaced attribute bag, an explicit
  field-definition and faceting document, or a deliberately deferred filtering
  capability. This requires dedicated design work before a public API is fixed.
- When the same root update graph occurs in multiple selected catalogs, should
  discovery return catalog-scoped occurrences, merge them, select the resolver
  priority winner, or report ambiguity?
- What is the exact resolution response: direct successors only, ranked
  candidates, an ordered upgrade path, or all of these as distinct operations?
- How should catalog priority, selectors, deprecation, version constraints,
  pagination, sort order, and partial import state be exposed consistently?
- Does this require a generated OpenAPI artifact, a maintained specification,
  or both, and which Go dependency or local representation best fits the
  project?

## Constraints

- This remains a reusable Go library. It must not introduce a standalone CLI,
  application server, or Kubernetes cluster dependency.
- Public API and wire-contract changes require tests, including contract tests
  against representative FBC and catalog-v2 fixtures.
- Legacy `operator-framework` dependencies remain limited to the FBC adapter;
  the REST model and handler must not expose legacy types.
- The design should preserve the repository convention of inert data types and
  standalone conversion functions.
