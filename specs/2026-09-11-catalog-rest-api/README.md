---
status: in-progress
---
# Catalog REST API

## Summary

Provide an embeddable, versioned REST API and OpenAPI contract for catalog
discovery and bundle recommendation. Consumers construct an `http.Handler` with
a `catalogv1.StoreReader` and mount it in their own application. The library
does not listen on a socket, manage server lifecycle, define authentication, or
interact with a Kubernetes cluster.

The API exposes selected catalogs, catalog-scoped package occurrences, nested
channels, bundles, package icons, and ranked install or upgrade candidates. It
uses the existing catalog and resolver abstractions so HTTP transport code does
not reproduce format-specific graph semantics.

This work also adds the portable package and bundle metadata needed by the API
to the core catalog model. The initial implementation populates that metadata
from legacy FBC. The proposed `olm.package.v2` model informs which fields are
portable, but catalog-v2 ingestion and metadata query/filter APIs are explicitly
deferred.

## Design

### Library boundaries

The public HTTP implementation lives in `catalog/http`. Its constructor
accepts a `catalogv1.StoreReader` and returns an `http.Handler`. The reader is
the caller's outer catalog-policy boundary. Requests that operate across
catalogs may further narrow it with a Kubernetes label selector parsed with
`k8s.io/apimachinery/pkg/labels`; a request cannot broaden the reader supplied
to the handler.

The handler is a library component, not an application. It owns request
parsing, validation, domain-to-wire projection, pagination, and HTTP error
mapping. It does not own a listen address, TLS, authentication, authorization,
logging policy, outbound credentials, or shutdown behavior.

The Go package path is intentionally not independently versioned. Its public API
is coupled to the module's current catalog interfaces, so a `/v1` Go package
would not isolate consumers from changes to those interfaces. The module version
governs Go API compatibility, while the `/v1` HTTP path independently versions
the wire contract.

The incomplete `examples/catalog_server` prototype is replaced by a minimal
working composition example that opens a SQLite store, constructs the public
handler, and mounts it on an application-owned server.

### Core catalog model

Package roots and nested channels are distinct domain concepts even though both
have update-graph behavior. Add a `catalogv1.Package` interface that embeds
`catalogv1.UpdateGraph` and exposes package-only metadata and icon retrieval.
Change `Catalog.ListPackages` and `Catalog.GetPackage` to return `Package`
values. Nested graphs remain `UpdateGraph` values and are called channels by the
wire API.

Package metadata is a typed value returned as `(PackageMetadata, error)` so
lazy stores can report read or decode failures. It contains the portable
presentation fields common to FBC and the proposed catalog-v2 model:

- display name
- short description and description
- provider name and optional HTTP(S) URL
- maintainers with names and optional email addresses
- keywords
- optional source repository HTTP(S) URL
- whether an icon is available

Bundle release metadata is a typed value returned as `(BundleMetadata, error)`
from `bundlev1.Bundle`. It contains optional media type and release timestamp.
Identity remains on the existing interfaces: graph names, bundle ID,
name/version/release, and URI are not duplicated in metadata.

The existing optional `catalogv1.Deprecated` capability remains the source of
package, channel, and bundle deprecation messages. This avoids replacing a
working format-neutral mechanism or adding otherwise-empty channel metadata.

Package icon access is lazy. `Package.Icon(ctx)` returns an `Icon` value and an
error, with absence represented explicitly. The initial `Icon` type provides
streamable content and its media type, and owns any required close operation.
It is a concrete result type that can gain another source representation, such
as an icon location, without changing the `Package.Icon` method signature.

The initial HTTP icon endpoint supports only streamable content: it writes the
declared media type and copies the stream to the response, or returns not found
when the package has no icon. Opening failures are Problem Details responses. A
copy failure after response headers have been sent terminates the response; the
handler cannot replace a partially written body with a JSON error.

These are intentional breaking changes to the current catalog and bundle
interfaces. All in-repository implementations and test doubles are updated
directly, without compatibility shims. FBC's importer extracts portable fields
as part of normal import. The SQLite implementation may retain normalized
values as internal properties, but callers see only typed interfaces.

### Resource identity and terminology

The wire API uses the terms **package**, **channel**, and **bundle**.

- A package occurrence is scoped by catalog name and package name. Discovery
  does not merge or discard duplicate package names across catalogs.
- Every child update graph, including a child of another child, is a channel.
- A channel is identified by an ordered path of graph-name segments relative to
  its package root. Responses represent the path as a string array. Channel
  resource URLs encode the path as one colon-delimited path parameter beneath
  `/channels/`, so `stable:1:2` represents `["stable", "1", "2"]`. A colon is
  reserved for this wire encoding and is invalid within an individual graph-name
  segment. This encoding does not make the domain model or catalog formats
  assume a fixed nesting depth.
- A bundle is identified within its catalog by bundle ID and also reports its
  package name, semantic version, and release.

Package discovery is sorted by package name ascending, catalog priority
descending for equal package names, and catalog name ascending for equal names
and priorities. Catalogs, channels, and bundles have documented deterministic
sort keys in the OpenAPI contract.

### HTTP resources

The handler serves the following versioned resource families beneath `/v1`:

- `GET /v1/catalogs` lists catalog metadata. It accepts catalog selector,
  cursor, and limit parameters.
- `GET /v1/catalogs/{catalog}` returns one catalog's metadata.
- `GET /v1/packages` lists catalog-scoped package occurrences across selected
  catalogs. It accepts catalog selector, cursor, and limit parameters.
- `POST /v1/recommendations/{package}` returns ranked install or
  immediate-upgrade candidates for the specified package. It accepts catalog
  selector, cursor, and limit query parameters plus recommendation criteria in
  a JSON body.
- `GET /v1/catalogs/{catalog}/packages/{package}` returns package detail.
- `GET /v1/catalogs/{catalog}/packages/{package}/channels` lists all descendant
  channels with their path arrays.
- `GET /v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}`
  returns one nested channel identified by its path segments.
- `GET /v1/catalogs/{catalog}/packages/{package}/bundles` lists bundles in the
  package root graph. For a composite package, this is the union across all
  descendant channels, matching `UpdateGraph.ListBundles`.
- `GET /v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles`
  lists bundles in the addressed channel graph. For a composite channel, this
  is the union across all its descendant channels.
- `GET /v1/catalogs/{catalog}/packages/{package}/bundles/{bundleID}` returns one
  bundle visible from the package.
- `GET /v1/catalogs/{catalog}/packages/{package}/icon` streams available icon
  content or returns not found.

All collection responses use summary wire types. Detail responses add the
remaining portable fields and links, but never expose Go interface values,
legacy FBC types, raw source properties, or proposed catalog-v2 types.
Bundle collections do not accept channel-path query parameters. A client that
wants bundles from several specific channels requests each channel's bundle
collection separately, with an independent cursor and limit for each request.

### Recommendation semantics

The recommendation request partitions inputs by purpose:

- the path supplies the required package name
- the query supplies the optional catalog label selector, cursor, and limit
- the JSON body supplies recommendation criteria

The JSON body contains:

- optional current bundle identity; omission means initial installation
- optional channel paths; omission means all channels
- optional semantic-version constraint; omission means all versions
- optional `upgradeConstraintPolicy`, with `CatalogProvided` as the default and
  `SelfCertified` as the other initial value

The catalog selector uses the same query parameter and syntax as
`GET /v1/packages`; omission means all catalogs visible through the handler's
base reader. Cursor and limit are also query parameters because they control
the response collection rather than recommendation semantics. To continue a
recommendation collection, a client repeats the same JSON body with the returned
cursor. The cursor is bound to both the selector and the recommendation body.

Define the two policy values locally rather than importing operator-controller
API types. Upgrade constraint policy belongs to this library's resolution
domain, so the HTTP package should not depend on the downstream ClusterExtension
API. If both APIs eventually require a single source of truth, their type
ownership and conversion boundary can be reconciled separately.

The HTTP layer maps the policy onto the existing resolver API. With
`CatalogProvided` and a current bundle, it supplies
`resolverv1.WithSuccessorsOf(current)` so resolution asks the selected graphs
for immediate successors. With `SelfCertified`, it omits that option so
resolution lists all bundles satisfying the package, channel, version, catalog,
and deprecation constraints. Both policies prefer non-deprecated bundles while
retaining deprecated candidates after them. No resolver policy option is added
until another policy requires behavior that cannot be expressed by the existing
options.

The response contains the selected catalog and package occurrence plus the
resolver's ordered candidates. Successor eligibility is defined by each
selected graph's `Successors` implementation; the current resolver deduplicates
those candidates and orders them by version and optional deprecation preference.
The handler preserves that order.

Future requirements may involve multi-hop path update recommendations, including
edge weights or shortest-path-style ranking. The current `UpdateGraph.Successors`
interface exposes only bundles, and the resolver replaces iterator order with
its own sorting, so those requirements would need an explicit change to the core
graph/resolver boundary. The REST response can still remain an ordered list of
bundles while that internal API evolves. This work returns immediate successors
only and does not compute paths in the HTTP layer.

Recommendation evaluates selected catalogs in descending priority groups. For
each group, it checks every catalog in that group for the requested package. A
read error from any catalog in the group fails the request, more than one match
is an ambiguity error, and exactly one match selects that package occurrence.
After selecting a package, recommendation does not inspect package content in
lower-priority groups. It advances to the next group only when every catalog in
the current group is readable and none contains the package. Selecting a package
from a lower-priority catalog therefore requires that no higher-priority
selected catalog contains it, typically enforced with a catalog selector.

Recommendations are a separate multi-catalog operation because they select one
package occurrence according to resolver priority. Package discovery returns
all matching catalog-scoped occurrences and never collapses them by priority;
each occurrence links to its catalog-scoped package resource.

Catalog and graph read failures are not treated as absence. Introduce and use a
typed not-found error where needed so the resolver can distinguish a catalog
that lacks the package from a catalog that cannot be read. For recommendation,
the failure boundary is the priority group currently being evaluated; unreadable
lower-priority catalog content is irrelevant once a higher-priority package has
been selected.

### Pagination and consistency

Collection endpoints and recommendation candidates use stateless opaque
cursors. The default limit is 50 and the maximum is 200. A cursor encodes the
last stable sort key, a hash of the normalized request inputs, and a compact
hash of the selected catalog snapshot. The catalog snapshot hash is computed
over one canonical representation of every selected catalog's name, labels,
digest, and priority, sorted by catalog name. Label keys are also sorted before
hashing so map iteration cannot affect the result. The cursor does not embed the
complete catalog snapshot, so its size does not grow with catalog content or the
number of returned resources. Encoding is an implementation detail and clients
must not inspect or construct tokens.

Cursor encoding has no required version field or compatibility guarantee across
handler upgrades. If a handler cannot decode a cursor produced by an earlier
deployment, it returns an invalid-cursor Problem Details response and the client
restarts pagination from the first page. Cursors are continuation state, not
durable API resources.

On continuation, the handler recomputes the normalized request and selected
catalog snapshot hashes and rejects a token if either changed. A stale cursor
returns a conflict Problem Details response.
This prevents silent duplication or omission across catalog replacement without
requiring server-side cursor sessions.

The handler finishes domain reads needed for a response before writing success
headers. Discovery operations that aggregate all selected catalogs fail if any
required catalog read fails. Recommendation follows the priority-group failure
boundary described above. Whether partially imported content is readable
remains a decision of the importer and store, not the HTTP or resolver layers.

### Errors and OpenAPI

Errors use `application/problem+json` following RFC 9457. Stable problem types
cover malformed input, invalid selectors or constraints, unsupported policy,
not found, invalid or stale cursor, equal-priority ambiguity, method not allowed,
and internal catalog failures. Validation errors identify invalid fields without
leaking internal database or filesystem details.

A checked-in, spec-first OpenAPI YAML document is the authoritative HTTP
contract. Contract tests validate representative requests, responses, content
types, status codes, and schemas against it. Adding OpenAPI-driven Go generation
is a follow-up item; this work does not add annotation, reflection, or generated
binding dependencies.

### Testing design

Tests cover behavior exhaustively in the package that owns it. Tests of a
consuming package cover only input translation, output preservation, and
boundary error mapping, using the minimum representative domain scenarios
needed to prove those responsibilities. In particular, HTTP tests do not repeat
the catalog store or resolver behavioral matrices.

Catalog implementation tests own storage and retrieval, selector behavior,
package and graph lookup, composite graph behavior, metadata and icon
persistence, typed not-found outcomes, and FBC-to-domain conversion. Resolver
unit tests own priority-group selection, absence versus read-error handling,
ambiguity, the lower-priority read boundary, graph and version filtering,
successor delegation, deduplication, candidate ordering, and deprecation
preference.

Handler unit tests use `httptest` with a fake `catalogv1.StoreReader`; they do
not use SQLite databases or FBC fixtures. They own routing and method handling,
path/query/body decoding and validation, translation to catalog and resolver
inputs, domain-to-wire projection, handler-defined collection sorting,
pagination and cursor behavior, preservation of resolver candidate order,
Problem Details mapping, response headers and content types, icon streaming,
and completing required reads before writing success headers. Recommendation
tests use only representative resolver scenarios needed to prove policy and
constraint translation, order preservation, and HTTP error mapping rather than
retesting resolution semantics.

Small catalog-domain fakes shared by resolver and HTTP tests live in
`internal/util/test`, alongside the existing bundle test values. They implement
only the production interfaces and the minimal failure injection or call
observation needed by tests; they do not form a scenario DSL and contain no
HTTP-specific assertions. Request builders, response decoders, Problem Details
assertions, OpenAPI helpers, and stream-failure helpers remain local to
`catalog/http` tests. Package-specific helpers remain local unless a second root
package demonstrates concrete reuse.

### Deferred work

The following are deliberately outside this work item:

- importing or adapting `olm.package.v2`
- exposing raw, vendor-specific, or namespaced metadata
- metadata query fields, predicates, filtering, facets, and field definitions
- icon references and HTTP redirect or proxy behavior for referenced icons
- multi-hop upgrade path computation
- OpenAPI-to-Go or Go-to-OpenAPI generation
- server lifecycle, authentication, authorization, and application policy
- Kubernetes clients or ClusterExtension API dependencies
