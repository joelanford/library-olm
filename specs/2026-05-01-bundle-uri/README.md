---
status: in-progress
---
# Bundle URI

## Summary

Expand the `bundlev1.Bundle` interface with content location and structured identity methods, so that bundles returned from catalog queries carry enough information for callers to fetch their content via existing tools (e.g. `image/bundle/` handlers). Introduce `BundleID` as a simple string type for lookup keys, and change `Successors` to accept `BundleID` instead of a full `Bundle`.

## Design

### New types and interface (`bundle/v1/`)

```go
// BundleID is the unique identifier for a bundle within a catalog.
type BundleID string

// Bundle represents a versioned unit of content in a catalog.
type Bundle interface {
    ID() BundleID
    NameVersionRelease() NameVersionRelease
    URI() string
}
```

`NameVersionRelease` is updated: `BundleName` is renamed to `Name` and documented as the package name. The `Name()` method is removed (callers access the field directly). The combination of (package name, version, release) forms the structured identity of a bundle, while `BundleID` is the opaque unique key.

```go
type NameVersionRelease struct {
    // Name is the package name this bundle belongs to.
    Name    string
    Version semver.Version
    Release Release
}
```

`NameVersionRelease` no longer implements `Bundle` — it remains a pure identity value type with `Compare()`.

### Catalog API change (`catalog/v1/`)

`Successors` changes its `from` parameter from `Bundle` to `BundleID`:

```go
type UpdateGraph interface {
    Name() string
    ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error]
    Successors(ctx context.Context, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error]
}
```

This decouples successor lookup from the full Bundle interface — callers only need the opaque ID, not version/release/URI.

### FBC implementation changes

**Schema** — columns added:

| Table | New Column | Source |
|---|---|---|
| `raw_olm_bundle` | `image TEXT NOT NULL DEFAULT ''` | `declcfg.Bundle.Image` (raw, no scheme prefix) |
| `bundles` | `package_name TEXT NOT NULL DEFAULT ''` | from `raw_olm_bundle.package_name` |
| `bundles` | `uri TEXT NOT NULL DEFAULT ''` | `"docker://" + raw_olm_bundle.image` (via handler) |

**Ingest** — `parseBundle` stores `b.Image` as-is in the `image` column of `raw_olm_bundle`.

**Handler** — `insertBundles` reads `image` from `raw_olm_bundle`, prepends the `docker://` scheme, and writes the result as `uri` in the normalized `bundles` table alongside `package_name`.

**Query** — `yieldBundleRows` selects `id, package_name, version, release, uri` and constructs a concrete type implementing `Bundle`:

```go
type bundleRow struct {
    id          bundlev1.BundleID
    packageName string
    version     bsemver.Version
    release     bundlev1.Release
    uri         string
}

func (b bundleRow) ID() bundlev1.BundleID                      { return b.id }
func (b bundleRow) NameVersionRelease() bundlev1.NameVersionRelease { ... }
func (b bundleRow) URI() string                                  { return b.uri }
```

Successor queries use `string(from)` directly as the `from_bundle_id` lookup key.

### URI scheme

FBC bundles carry an OCI image reference (e.g. `quay.io/foo/bar:v1.0.0`). The URI stored in the database is prefixed with `docker://` to form a scheme-qualified URI: `docker://quay.io/foo/bar:v1.0.0`. This enables callers to dispatch to the appropriate fetcher by scheme without parsing conventions.

### Caller patterns

**List bundles with URIs:**
```go
for b, err := range pkg.ListBundles(ctx) {
    fmt.Printf("%s → %s\n", b.ID(), b.URI())
}
```

**Find successors by ID:**
```go
for next, err := range pkg.Successors(ctx, installed.ID()) {
    fmt.Printf("can upgrade to %s at %s\n", next.ID(), next.URI())
}
```

**Access version info:**
```go
nvr := b.NameVersionRelease()
fmt.Printf("package=%s version=%s\n", nvr.Name, nvr.Version)
```
