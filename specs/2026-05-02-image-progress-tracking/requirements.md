# Requirements

- `Handler` interface gains a `Discover(ctx, repo, desc, manifestBytes) ([]ocispecv1.Descriptor, error)` method
- `Discover` walks the image tree and returns all descriptors needed for unpack (manifests, configs, layers, subjects)
- `Discover` fetches the minimum necessary to discover the full graph (manifests and configs, not layer blobs)
- `Discover` does not write anything to disk
- `CachingRepository` gains a `CachedDescriptors() []ocispecv1.Descriptor` method
- `CachedDescriptors` returns descriptors for all content currently in the cache
- Progress tracking is a caller concern — callers wrap `Repository` with their own byte-counting logic
- `Repository` interface is unchanged — `FetchManifest` and `FetchBlob` remain as-is
- `ContainersImageClient` is renamed to `ContainersImageRepository`
- `Unpacker` is unchanged — orchestration is a caller concern
- All three handlers (FBC, RegistryV1, Helm) implement `Discover`
- `ManifestUnpacker` is removed — its logic moves to internal `ApplyLayers` function in `image/internal/ociutil`
- `DiscoverManifestDescriptors` and `ApplyLayers` (with filter types) are internal — not part of the public API
- All existing handler tests continue to pass
- All existing `CachingRepository` tests continue to pass

## Acceptance Criteria

- `handler.Discover(ctx, repo, desc, manifestBytes)` returns a complete descriptor list for any supported image type
- `Discover` with a `CachingRepository` caches manifests/configs that `Unpack` reuses without re-fetching
- `CachedDescriptors()` returns descriptors for cached content; callers can intersect with discovered descriptors
- FBC, RegistryV1, and Helm handlers produce identical unpacked output before and after adding `Discover`
- No `ManifestUnpacker` type exists after migration; internal `ApplyLayers` function replaces it
- `ContainersImageClient` type is renamed to `ContainersImageRepository`
- `Unpacker` has no progress or prefetch logic — callers compose the building blocks
- Test coverage does not decrease
