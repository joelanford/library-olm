# Implementation Plan

## 1. Rename ContainersImageClient to ContainersImageRepository

Rename the type and its constructor. Update all references. This is a standalone rename with no behavioral changes.

## 2. Add CachedDescriptors to CachingRepository

Add `CachedDescriptors() []ocispecv1.Descriptor` to `CachingRepository`. It iterates the in-memory manifest cache and the on-disk blob cache, returning descriptors for all cached content. Descriptors include digest, size, and media type where available.

## 3. Add Discover to Handler interface

Add `Discover(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) ([]ocispecv1.Descriptor, error)` to the `Handler` interface.

## 4. Implement Discover for FBCHandler

Walk the image tree:
- If index: choose platform, fetch child manifest
- Parse manifest: fetch config blob to read labels
- Return all discovered descriptors (the resolved descriptor, child manifests, config, all layers)

`Discover` fetches manifests and configs (small) but not layer blobs. It uses the same tree-walking logic as `Unpack` but without applying layers to disk.

## 5. Implement Discover for RegistryV1Handler

Same pattern as FBC: resolve index if needed, fetch manifest, fetch config to read labels, return all descriptors including layers.

## 6. Implement Discover for HelmChartHandler

Parse manifest, identify config blob, chart layer, and optional provenance layer. Return all descriptors. Config may need to be fetched to identify provenance requirements.

## 7. Remove ManifestUnpacker

Delete `ManifestUnpacker` and its associated methods. Keep `LayerFilter`, `CombineFilters`, `OnlyPaths`, `RewritePath`, and `AsCurrentUser`.

Replace with an internal `ApplyLayers(ctx, repo, manifestBytes, dest, filter)` function in `image/internal/ociutil` that handlers call directly. `DiscoverManifestDescriptors` and all filter types (`LayerFilter`, `CombineFilters`, `OnlyPaths`, `RewritePath`, `AsCurrentUser`) also live in this internal package.

## 8. Keep Unpacker unchanged

`Unpacker` stays focused on match → unpack. Progress tracking is a caller concern — callers wrap `Repository` with their own byte-counting logic and compose `Discover` and `CachedDescriptors` to orchestrate progress tracking as needed.

## 9. Update tests

- Update all `Handler` mock/fake implementations to add `Discover`
- Add unit tests for each handler's `Discover`: verify returned descriptors match what `Unpack` would fetch
- Add unit tests for `CachedDescriptors`: verify it reflects cached manifests and blobs
- Add unit tests for `DiscoverManifestDescriptors` (in `image/internal/ociutil`): verify it returns correct descriptors and caches config
- Verify `CachingRepository` singleflight handles concurrent prefetch + handler fetch without races
