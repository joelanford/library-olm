# Verification

## Implementation Correctness
- [ ] `FromFS` walks the filesystem via `declcfg.WalkMetasFS` and processes one blob at a time.
- [ ] Only `olm.package`, `olm.channel`, and `olm.bundle` schemas are ingested; others are ignored.
- [ ] Bundle version is extracted from the `olm.package` property and parsed via `blang/semver/v4`.
- [ ] SQLite schema has raw tables named by convention from FBC schema strings (raw_olm_package, raw_olm_channel, raw_olm_channel_entry, raw_olm_bundle) and normalized tables (graphs with self-referential parent_id, bundles, graph_bundles join table, successors).
- [ ] Root graphs (packages) have NULL parent_id; child graphs (channels) reference their parent.
- [ ] Bundles are deduplicated in the `bundles` table; `graph_bundles` is a many-to-many join.
- [ ] `PackageSchemaHandler` interface is defined with `Schema()`, `CompanionSchemas()`, and `Normalize()` methods.
- [ ] An `olm.package` handler is registered and processes its neighborhood correctly.
- [ ] The `olm.package` handler validates: known references, no duplicates, all channel entries have bundle blobs.
- [ ] Validation errors are returned from `FromFS`, not deferred.
- [ ] Successor computation handles `replaces`, `skips`, and `skipRange` correctly.
- [ ] `skipRange` uses `blang/semver/v4` range parsing to match bundle versions within the channel.
- [ ] `ListPackages` returns `CompositeUpdateGraph` values.
- [ ] `GetPackage` returns a `CompositeUpdateGraph` or an appropriate error for unknown packages.
- [ ] `ListGraphs`/`GetGraph` on a `CompositeUpdateGraph` return per-graph `UpdateGraph`s.
- [ ] `ListBundles` on a graph yields only bundles in that graph.
- [ ] `ListBundles` on a package yields the deduplicated union across all graphs.
- [ ] `Successors` on a graph yields successors within that graph only.
- [ ] `Successors` on a package yields the deduplicated union of successors across all graphs.
- [ ] All yielded bundles are `bundlev1.NameVersionRelease` values.
- [ ] `Close` removes the temporary SQLite file and releases resources.
- [ ] Adding a new `PackageSchemaHandler` does not require changes to the ingest or query layers.

## Project Conventions
- [ ] No FBC-specific types or terminology leak into `catalog/v1/` or `bundle/v1/`.
- [ ] Internal implementation details are in `catalog/fbc/internal/`.
- [ ] Public API surface in `catalog/fbc/` is minimal: `Catalog`, `FromFS`, `Close`.
- [ ] Uses `modernc.org/sqlite` (no cgo).
- [ ] Uses `operator-framework/operator-registry/alpha/declcfg` for FBC parsing only — no dependency on `operator-registry/alpha/model`.
- [ ] No cluster dependencies (no kubeconfig, no kube client, no controller-runtime).
- [ ] `make ci` passes.
