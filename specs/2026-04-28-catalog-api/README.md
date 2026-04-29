---
status: idea
---
# Catalog Public API and FBC Implementation

Define a public API for catalogs (types, querying, indexing) and implement that API with File-Based Catalog (FBC) as the backing format.

## Key design constraint

FBC-specific concepts (channels, replaces/skips/skipRange update edges) must not leak into the public catalog API. The public API should be format-agnostic — FBC is one implementation, not the model itself.

**Inspiration: OCI's layered approach.** OCI defines a common API (descriptors, media types, content-addressable storage), and then artifacts, indexes, and manifests bring different semantics on top — particularly the opaque semantics of artifacts, where the registry stores and serves content without understanding its structure. The catalog API could follow a similar pattern: a common interface for storing, querying, and iterating catalog entries, with format-specific semantics (like FBC channels and upgrade edges) layered on top as an implementation detail or an optional typed view.

**Channels as multiplexing.** An FBC package with multiple channels is analogous to an OCI index (a single reference that fans out to multiple concrete things), while a package with a single channel is analogous to an OCI manifest (a single reference that maps directly to content). This framing could help inform how the public API models the relationship between packages and their contents without hard-coding channel semantics.

## Spec structure

This work item should produce two separate specs during refinement:

1. **Public catalog API** — the format-agnostic types and interfaces (querying, indexing, iterating catalog entries, predecessor/successor relationships).
2. **FBC implementation** — the FBC-specific implementation of the public API, covering `olm.package`, `olm.channel`, `olm.bundle`, and `olm.deprecations`.

## Design notes

**Update edges: relationships, not mechanisms.** The public API should express universal graph relationships — predecessors and successors — without prescribing how they are computed. FBC's replaces/skips/skipRange are mechanisms for defining those relationships in one particular format; the public API surfaces the computed result. Leave room in the design for conditional predecessors/successors (edges that depend on runtime context or caller-supplied constraints).
