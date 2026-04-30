---
status: idea
---
# Catalog FBC Implementation

## Summary

Implement the `catalogv1.Catalog` interfaces for the File-Based Catalog (FBC) format. This builds on the public API defined in `specs/2026-04-28-catalog-api/` by providing concrete implementations that parse FBC declarative config data and expose it through the format-agnostic catalog interfaces.

## Key Concepts

| FBC concept | Catalog API mapping |
|---|---|
| Catalog (collection of packages) | `catalogv1.Catalog` |
| `olm.package` | `catalogv1.CompositeUpdateGraph` (returned by `GetPackage`) |
| `olm.channel` | Child `catalogv1.UpdateGraph` within the composite |
| `olm.bundle` | `bundlev1.Bundle` interface (name + version + release) |
| replaces/skips/skipRange edges | Computed into `Successors` results |
| `olm.deprecations` | Deferred — not modeled initially |

## Open Questions

- Where does the FBC implementation live? `catalog/fbc/` or `internal/catalog/fbc/`?
- How to handle FBC bundle properties and arbitrary metadata beyond name/version/release?
- Should the FBC implementation support incremental/streaming catalog loading?
- How to compute the upgrade graph from replaces/skips/skipRange edges efficiently?
