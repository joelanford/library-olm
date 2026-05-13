---
status: done
---
# Image Progress Tracking

## Summary

Add a `Discover` method to the `Handler` interface that returns the full set of OCI descriptors needed to unpack an image. This enables progress tracking with a known total, concurrent prefetching into the cache, and a clean separation between tree exploration and content processing. The `Repository` interface is unchanged.

## Design

### Problem

An OCI image is a tree of artifacts — indices, manifests, and blobs. To unpack an image, a handler traverses this tree, deciding at each level which children to fetch. The total size of content to download isn't known upfront — it's discovered progressively as the handler parses indices and manifests.

The current `Repository` interface has separate `FetchManifest` and `FetchBlob` methods called one at a time. There is no way to know the total size of content before unpacking begins, which makes meaningful progress bars impossible.

### Handler Discover Method

Add a `Discover` method to the `Handler` interface:

```go
type Handler interface {
    Name() string
    Matches(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) (bool, error)
    Discover(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) ([]ocispecv1.Descriptor, error)
    Unpack(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte, dest string) error
}
```

`Discover` walks the image tree — resolving indices, parsing manifests, reading configs — and returns the complete set of descriptors the handler would need to unpack. It fetches the minimum necessary to discover the full graph (manifests and configs, not layer blobs). It does not write anything to disk.

When used with a `CachingRepository`, everything fetched during `Discover` is cached and reused by `Unpack` for free.

### CachingRepository: CachedDescriptors

Add a method to `CachingRepository` that returns descriptors for cached content:

```go
func (c *CachingRepository) CachedDescriptors() []ocispecv1.Descriptor
```

This is a generic API — it returns what's in the cache without knowing why the caller is asking. The caller can intersect with the discovered descriptor set to determine how much has already been fetched.

### Caller Orchestration

The library provides building blocks; callers compose them to orchestrate progress tracking and prefetching. Progress tracking is a caller concern — callers wrap `Repository` with their own byte-counting logic, since progress reporting needs vary widely (TUI progress bars, logging, metrics, etc.).

```go
inner, err := image.NewContainersImageRepository(ref)
cached, err := image.NewCachingRepository(inner)

// Discover — walks tree, manifests/configs get cached
descs, err := handler.Discover(ctx, cached, desc, manifestBytes)

// Caller implements progress tracking by wrapping the repository
total := sumSizes(descs)
progress := myProgressRepository(cached, total, onProgress)

// Prefetch blobs concurrently into the cache
prefetchBlobs(ctx, cached, descs)

// Unpack — reads from cache, progress tracks bytes consumed
err = handler.Unpack(ctx, progress, desc, manifestBytes, dest)
```

Callers that want concurrent prefetching can implement it on top of `CachingRepository` — the singleflight deduplication ensures concurrent fetches for the same digest are safe.

### UX

Two phases visible to the user:

1. **Discovering** — spinner or indeterminate progress while `Discover` walks the tree
2. **Unpacking** — progress bar from `alreadyFetched/total` to `total/total`, with a known fixed total

### Affected Types

| Type | Change |
|---|---|
| `Repository` interface | **Unchanged** |
| `ContainersImageClient` | Rename to `ContainersImageRepository` |
| `CachingRepository` | Add `CachedDescriptors()` method |
| `Handler` interface | Add `Discover` method |
| `Unpacker` | **Unchanged** — orchestration is a caller concern |
| `FBCHandler` | Add `Discover` implementation |
| `RegistryV1Handler` | Add `Discover` implementation |
| `HelmChartHandler` | Add `Discover` implementation |
| `ManifestUnpacker` | Remove — its logic moves to internal `ApplyLayers` function in `image/internal/ociutil` |

### Prior Art

- **`go-containerregistry` (crane)**: `chan<- v1.Update` — per-blob progress via channel. No total upfront.
- **`containers/image` (podman/skopeo)**: `chan types.ProgressProperties` + `ProgressInterval` — per-blob progress events. No aggregate view.
- **`oras-go`**: `PreCopy`/`PostCopy` per-descriptor callbacks during DAG traversal. Closest to knowing the graph ahead of time, but at descriptor level, not byte level.

None provide a known total before content transfer begins.
