---
status: in-progress
---
# FBC Test Utility Library

## Summary

Internal test utility package providing a fluent builder for constructing in-memory FBC catalog filesystems (`fstest.MapFS`). Mirrors the existing `bundlefs` builder pattern for registry+v1 bundles. The builder auto-derives bundle names, images, and properties from minimal input (package name + version), with overrides for edge cases.

## Design

### Package location

`catalog/fbc/internal/testing/catalogfs/`

Internal to the `catalog/fbc/` subtree, following the same pattern as `bundle/registry/v1/internal/util/testing/bundlefs/`.

### Builder API

```go
package catalogfs

import "testing/fstest"

// CatalogFSBuilder constructs an FBC catalog filesystem.
type CatalogFSBuilder interface {
    WithPackage(name string) CatalogFSBuilder
    WithChannel(pkg, name string, entries ...ChannelEntry) CatalogFSBuilder
    WithBundle(pkg, version string, opts ...BundleOption) CatalogFSBuilder
    WithCustom(pkg, schema, name string, fieldKVs ...any) CatalogFSBuilder
    Build() fstest.MapFS
}

// Builder returns a new CatalogFSBuilder.
func Builder() CatalogFSBuilder
```

### Channel entries

```go
// ChannelEntry represents an entry in a channel.
type ChannelEntry struct { /* unexported fields */ }

// Entry creates a channel entry for the bundle at the given version.
// During Build, the entry's bundle name is derived as "<package>.v<version>"
// using the package from the containing WithChannel call.
func Entry(version string, opts ...EntryOption) ChannelEntry

type EntryOption func(*ChannelEntry)

// Replaces sets the replaces field. The version is resolved to
// "<package>.v<version>" during Build.
func Replaces(version string) EntryOption

// Skips sets the skips list. Each version is resolved to
// "<package>.v<version>" during Build.
func Skips(versions ...string) EntryOption

// SkipRange sets the skipRange field (raw semver range string, not resolved).
func SkipRange(r string) EntryOption
```

### Bundle options

```go
type BundleOption func(*bundle)

// WithName overrides the auto-derived bundle name (default: "<package>.v<version>").
func WithName(name string) BundleOption

// WithImage overrides the auto-derived image (default: "quay.io/<package>/bundle:v<version>").
func WithImage(image string) BundleOption

// WithRelease sets the release field in the olm.package property.
func WithRelease(release string) BundleOption
```

### Custom blobs

```go
// WithCustom adds a custom blob with schema, package, and name fields,
// plus additional fields from alternating key-value pairs.
// Panics if len(fieldKVs) is odd or any key is not a string.
func WithCustom(pkg, schema, name string, fieldKVs ...any) CatalogFSBuilder
```

Useful for injecting unknown schema blobs or malformed blobs in error-path tests.

### Defaults

| Field | Default | Override |
|---|---|---|
| Bundle name | `<package>.v<version>` | `WithName` |
| Bundle image | `quay.io/<package>/bundle:v<version>` | `WithImage` |
| Bundle release | empty | `WithRelease` |
| Entry bundle name | `<package>.v<version>` | — (follows bundle name convention) |
| Entry replaces | `<package>.v<version>` | — |

### Output format

`Build()` returns an `fstest.MapFS` with one file per blob, organized by package directory:
- `<package>/olm.package.json`
- `<package>/olm.channel.<channel-name>.json`
- `<package>/olm.bundle.<bundle-name>.json`

If two blobs target the same file path (e.g. a `WithBundle` and a `WithCustom` with schema `olm.bundle` for the same name), the second blob is appended to the file with a newline separator — producing a multi-blob file that `WalkMetasFS` handles natively.

`Build()` panics on internal errors (JSON marshal failures), following the `bundlefs` precedent. These indicate programmer error in the test setup.

### Example usage

```go
fs := catalogfs.Builder().
    WithPackage("my-operator").
    WithChannel("my-operator", "stable",
        catalogfs.Entry("1.0.0"),
        catalogfs.Entry("1.1.0", catalogfs.Replaces("1.0.0")),
    ).
    WithChannel("my-operator", "fast",
        catalogfs.Entry("1.0.0"),
        catalogfs.Entry("1.2.0", catalogfs.Replaces("1.0.0")),
    ).
    WithBundle("my-operator", "1.0.0").
    WithBundle("my-operator", "1.1.0").
    WithBundle("my-operator", "1.2.0").
    Build()
```

### Scope boundaries

The builder targets well-formed FBC catalogs for the happy path. `WithCustom` handles error-path test cases that need malformed or non-standard blobs (wrong field types, missing properties, unknown schemas).
