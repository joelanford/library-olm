---
status: ready
---
# Semver Successor Constraints

## Summary

Add semver-based successor constraints to the catalog v1 API. Today, all successor edges are
explicit `(from, to)` tuples materialized in `content_successors` at ingest time. This works for
`replaces` and `skips` but requires expanding every version-range relationship (`skipRange`) into
individual rows. The new mechanism stores a Masterminds semver constraint string per bundle per
graph and evaluates it dynamically at query time: when `Successors(ctx, from)` is called, the
`from` bundle's version is checked against all constraints in the graph, and matching bundles are
returned alongside explicit edges (union semantics).

## Design

### Schema

A new `content_predecessor_constraints` table with primary key `(graph_id, bundle_id)` — one
constraint per bundle per graph. The constraint describes which predecessor versions this bundle
accepts upgrades from. Complex disjunctions use Masterminds `||` syntax within the constraint
string.

```sql
CREATE TABLE content_predecessor_constraints (
    graph_id           INTEGER NOT NULL,
    bundle_id          INTEGER NOT NULL,
    version_constraint TEXT NOT NULL,
    PRIMARY KEY (graph_id, bundle_id),
    FOREIGN KEY (graph_id) REFERENCES content_graphs(id),
    FOREIGN KEY (bundle_id) REFERENCES content_bundles(id)
);

CREATE INDEX idx_content_predecessor_constraints_lookup
    ON content_predecessor_constraints(graph_id);
```

`ContentSchemaVersion` bumps from 1 to 2. The content layer's "drop and rebuild on mismatch"
semantics handle migration automatically.

### Writer

New method on the `Writer` interface:

```go
AddPredecessorConstraint(graph GraphID, bundleID, constraint string) error
```

Validates the constraint string with `semver.NewConstraint()` from `Masterminds/semver/v3` at
write time — returns an error if unparseable.

### Successors signature change

The `Successors` method on `UpdateGraph` and `CompositeUpdateGraph` changes from:

```go
Successors(ctx context.Context, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error]
```

to (where `mmsemver` is `github.com/Masterminds/semver/v3`):

```go
Successors(ctx context.Context, id bundlev1.BundleID, version mmsemver.Version) iter.Seq2[bundlev1.Bundle, error]
```

The caller provides both the bundle ID and the version of the installed bundle. Neither is
derivable from the other in all cases: dangling `replaces`/`skips` in FBC create phantom bundles
with an ID but no version, and the installed bundle may not exist in the catalog at all (so its
version can't be looked up by ID).

- `id` is used for explicit edge lookup in `content_successors`.
- `version` is used for predecessor constraint evaluation against `content_predecessor_constraints`.

### Query-time evaluation

`Successors()` returns the **union** of explicit edges and constraint matches, deduplicated by
bundle ID.

For a `Successors(ctx, id, version)` call:

1. Query explicit edges from `content_successors` using `id` (existing behavior).
2. Query all `(bundle_id, version_constraint)` rows from `content_predecessor_constraints` for the
   relevant graph(s).
3. For each row, parse the constraint with `semver.NewConstraint()` and check against `version`.
   If the constraint is unparseable, yield an error.
4. Collect matching bundles and yield them, skipping any already yielded from step 1.

Both the direct (leaf graph) and descendant (composite graph) query paths are updated.

### Dependency promotion

`Masterminds/semver/v3` moves from indirect to direct dependency.

### Performance

Constraint evaluation happens in Go, not SQL — each `Successors()` call iterates all constraint
rows for the relevant graph(s). This is acceptable for typical catalog sizes (hundreds of bundles
per graph, not millions). No optimization (caching, pre-compilation) is included in scope.
