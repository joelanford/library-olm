---
status: in-progress
---
# Catalog FBC Implementation

## Summary

Implement the `catalogv1.Catalog` interface for the File-Based Catalog (FBC) format. `FromFS` walks an `fs.FS` containing FBC JSON/YAML blobs, stores structured data into a SQLite database, then dispatches per-package-schema handlers that validate, compute successor graphs, and populate normalized tables. The resulting catalog answers all `catalogv1` queries from the normalized tables.

## Design

### Two-phase architecture

**Phase 1 — Ingest (format-agnostic):** Walk the FS via `declcfg.WalkMetasFS` (with `WithConcurrency(runtime.NumCPU())` for parallel blob parsing), parse each blob, and insert structured fields into **raw tables** in the database. Each row retains its schema type. This phase does not interpret relationships between schemas.

> **Concurrency note:** Multiple goroutines will call into the same `*sql.DB` concurrently. Go's `database/sql` pool and SQLite's internal locking make this safe, but transaction strategy matters for performance — see the implementation plan for details.

**Phase 2 — Normalize (per-package-schema):** Iterate over packages in the database. For each package, look up its schema (e.g. `olm.package`) and dispatch to a registered **package schema handler**. The handler:
- Knows which companion schemas belong to its neighborhood (e.g. `olm.package` → `olm.channel`, `olm.bundle`, `olm.deprecations`)
- Validates the neighborhood (referential integrity, no duplicates, etc.)
- Computes successor edges from format-specific upgrade semantics
- Writes results into **normalized tables** that back the `catalogv1` query API

This separation means adding support for a new package schema (e.g. `olm.package.v2` with `olm.channel.v2`, `olm.bundle.v2`) only requires registering a new handler — the ingest phase and query layer don't change.

### Entry point (`catalog/fbc/`)

```go
// Catalog is a catalogv1.Catalog backed by FBC data stored in SQLite.
// Call Close when done to clean up the temporary database file.
type Catalog struct { ... }

// FromFS walks fsys, parsing FBC blobs via declcfg.WalkMetasFS, and
// loads the structured data into a SQLite database. After loading,
// it dispatches per-package-schema handlers that validate and compute
// successor graphs into normalized tables. Only one blob is in memory
// at a time during the walk phase.
//
// The database is stored in a temporary file. Call Close to remove it.
func FromFS(ctx context.Context, fsys fs.FS) (*Catalog, error) { ... }

// Close releases database resources and removes the temporary file.
func (c *Catalog) Close() error { ... }
```

`Catalog` implements `catalogv1.Catalog`. `GetPackage` returns a `CompositeUpdateGraph` (since FBC has channels). `ListPackages` yields `CompositeUpdateGraph`s.

### Package schema handler

```go
// PackageSchemaHandler normalizes a package and its companion blobs
// from the raw tables into the normalized tables.
type PackageSchemaHandler interface {
    // Schema returns the package schema this handler processes (e.g. "olm.package").
    Schema() string

    // CompanionSchemas returns the set of blob schemas that belong to this
    // package schema's neighborhood (e.g. ["olm.channel", "olm.bundle"]).
    CompanionSchemas() []string

    // Normalize validates the package's neighborhood in the raw tables
    // and populates the normalized tables (bundles, channels/graphs,
    // successor edges). Called once per package during phase 2.
    Normalize(ctx context.Context, db *sql.DB, packageName string) error
}
```

The initial implementation registers a single handler for `olm.package`. Future package schemas register additional handlers.

### Database schema

**Raw tables** (populated during phase 1 ingest):

Raw table names are derived from the FBC schema string: replace `.` with `_`, prefix with `raw_` (e.g. `olm.package` → `raw_olm_package`). This convention ensures predictable, collision-free names when new schemas are added.

| Table | Columns | Purpose |
|---|---|---|
| `raw_olm_package` | name | `olm.package` blobs |
| `raw_olm_channel` | name, package_name | `olm.channel` blobs |
| `raw_olm_channel_entry` | channel_name, package_name, bundle_name, replaces, skips, skip_range | Channel entry details |
| `raw_olm_bundle` | name, package_name, version | `olm.bundle` blobs |

**Normalized tables** (populated during phase 2 by handlers):

The graph structure supports arbitrary nesting via a self-referential `graphs` table. For `olm.package`, this is two levels deep (package → channels), but the schema supports future formats with deeper hierarchies.

| Table | Columns | Purpose |
|---|---|---|
| `graphs` | id (PK), name, parent_id (FK nullable → graphs.id) | Recursive graph tree. Root graphs (packages) have NULL parent. Children (e.g. channels) reference their parent. |
| `bundles` | id (PK), name, version, release | Bundle identity (deduplicated across graphs) |
| `graph_bundles` | graph_id (FK → graphs.id), bundle_id (FK → bundles.id) | Many-to-many join: which bundles belong to which graphs |
| `successors` | graph_id (FK → graphs.id), from_bundle_id (FK → bundles.id), to_bundle_id (FK → bundles.id) | Precomputed successor edges within a graph |

### Query implementation

All queries read from the normalized tables. Each query type holds a `graph_id` and queries against it:

- **`ListPackages`** — `SELECT` root graphs (where `parent_id IS NULL`), yield one `CompositeUpdateGraph` per row
- **`GetPackage`** — `SELECT` root graph `WHERE name = ? AND parent_id IS NULL`
- **`ListGraphs`** — `SELECT` child graphs `WHERE parent_id = ?`
- **`GetGraph`** — `SELECT` child graph `WHERE parent_id = ? AND name = ?`
- **`ListBundles`** (leaf graph) — `SELECT` from `graph_bundles JOIN bundles WHERE graph_id = ?`
- **`ListBundles`** (composite graph) — `SELECT DISTINCT` bundles across all descendant graphs
- **`Successors`** (leaf graph) — `SELECT` from `successors JOIN bundles WHERE graph_id = ? AND from_bundle_id = ?`
- **`Successors`** (composite graph) — `SELECT DISTINCT` successors across all descendant graphs

### `olm.package` handler

The handler for `olm.package` validates and normalizes:

1. **Validate** — every `olm.channel` references a known package; every channel entry references a known `olm.bundle`; every `replaces` target exists; no duplicates
2. **Populate `graphs`** — one root graph per package (parent_id NULL), one child graph per `olm.channel` (parent_id → package graph)
3. **Populate `bundles`** — one row per unique bundle (deduplicated by name+version+release)
4. **Populate `graph_bundles`** — link each channel's entries to their bundle rows
5. **Compute successors** — from `replaces`, `skips`, and `skipRange` edges:
   - `replaces`: B is a successor of the bundle it replaces
   - `skips`: B is a successor of each bundle it skips
   - `skipRange`: B is a successor of every bundle in the channel whose version falls within the semver range

### Dependency choices

- **SQLite driver:** `modernc.org/sqlite` (pure Go, no cgo)
- **FBC parsing:** `operator-framework/operator-registry/alpha/declcfg` (reuse `WalkMetasFS`, `Meta`, schema constants, typed blob unmarshaling)
- **Semver range parsing** (for `skipRange`): `blang/semver/v4` (already a dependency)

### Internal organization (`catalog/fbc/internal/`)

All database access, schema definitions, handler interface, handler implementations, and query types live in `catalog/fbc/internal/`. The public `catalog/fbc/` package is a thin wrapper.

### How formats map to the catalog API

| Catalog API | FBC implementation |
|---|---|
| `catalogv1.Catalog` | `fbc.Catalog` (queries normalized SQLite tables) |
| `catalogv1.CompositeUpdateGraph` | One per package — child graphs are rows in `graphs` |
| `catalogv1.UpdateGraph` | One per graph (channel in `olm.package`) |
| `bundlev1.Bundle` | `bundlev1.NameVersionRelease` (identity only) |
