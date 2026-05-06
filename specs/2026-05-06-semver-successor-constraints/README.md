---
status: done
---
# Semver Successor Ranges

## Summary

Add semver-based predecessor ranges to the catalog v1 API. Today, all successor edges are
explicit `(from, to)` tuples materialized in `content_successors` at ingest time. This works for
`replaces` and `skips` but requires expanding every version-range relationship (`skipRange`) into
individual rows. The new mechanism stores a blang/semver range string per bundle per graph and
evaluates it dynamically at query time: when `Successors(ctx, fromID, fromVersion)` is called,
`fromVersion` is checked against all predecessor ranges in the graph, and matching bundles are
returned alongside explicit edges (union semantics).

## Design

### Schema

A new `content_predecessor_ranges` table with primary key `(graph_id, bundle_id)` — one
range per bundle per graph. The range describes which predecessor versions this bundle
accepts upgrades from. Complex disjunctions use blang `||` syntax within the range string.

```sql
CREATE TABLE content_predecessor_ranges (
    graph_id      INTEGER NOT NULL,
    bundle_id     INTEGER NOT NULL,
    version_range TEXT NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id),
    FOREIGN KEY (bundle_id) REFERENCES content_bundles(id)
);

CREATE INDEX idx_content_predecessor_ranges_lookup
    ON content_predecessor_ranges(graph_id);
```

`ContentSchemaVersion` bumps from 1 to 3. The content layer's "drop and rebuild on mismatch"
semantics handle migration automatically.

### Writer

New method on the `Writer` interface:

```go
AddPredecessorRange(graph GraphID, bundleID, versionRange string) error
```

Validates the range string with `bsemver.ParseRange()` from `blang/semver/v4` at
write time — returns an error if unparseable. Uses plain `INSERT` (duplicates are
prevented by FBC-level validation of unique channel entries).

### Successors signature change

The `Successors` method on `UpdateGraph` and `CompositeUpdateGraph` changes from:

```go
Successors(ctx context.Context, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error]
```

to:

```go
Successors(ctx context.Context, fromID bundlev1.BundleID, fromVersion bsemver.Version) iter.Seq2[bundlev1.Bundle, error]
```

The caller provides both the bundle ID and the version of the installed bundle. Neither is
derivable from the other in all cases: dangling `replaces`/`skips` in FBC create phantom bundles
with an ID but no version, and the installed bundle may not exist in the catalog at all (so its
version can't be looked up by ID).

- `fromID` is used for explicit edge lookup in `content_successors`.
- `fromVersion` is used for predecessor range evaluation against `content_predecessor_ranges`.

### Query-time evaluation

`Successors()` returns the **union** of explicit edges and range matches, deduplicated by
bundle ID.

For a `Successors(ctx, fromID, fromVersion)` call:

1. Query explicit edges from `content_successors` using `fromID` (existing behavior).
2. Query all `(bundle_id, version_range)` rows from `content_predecessor_ranges` for the
   relevant graph(s).
3. For each row, parse the range with `bsemver.ParseRange()` and check against `fromVersion`.
   If the range is unparseable, yield an error.
4. Yield matching bundles, skipping any already yielded from step 1.

Both the direct (leaf graph) and descendant (composite graph) query paths are updated.

### Performance

Range evaluation happens in Go, not SQL — each `Successors()` call iterates all range
rows for the relevant graph(s). This is acceptable for typical catalog sizes (hundreds of bundles
per graph, not millions). No optimization (caching, pre-compilation) is included in scope.
