# Verification

## Implementation Correctness

- [ ] `Repository` interface is unchanged — `FetchManifest` and `FetchBlob` remain as-is
- [ ] `ContainersImageClient` is renamed to `ContainersImageRepository`
- [ ] `CachingRepository` has `CachedDescriptors() []ocispecv1.Descriptor`
- [ ] `CachedDescriptors` returns descriptors for all cached manifests and blobs
- [ ] `Handler` interface has `Discover(ctx, repo, desc, manifestBytes) ([]ocispecv1.Descriptor, error)`
- [ ] FBCHandler `Discover` returns all descriptors needed for unpack (resolved desc, child manifests, config, layers)
- [ ] RegistryV1Handler `Discover` returns all descriptors needed for unpack
- [ ] HelmChartHandler `Discover` returns all descriptors needed for unpack
- [ ] `Discover` fetches only manifests and configs, not layer blobs
- [ ] `Discover` with `CachingRepository` caches fetched manifests/configs for reuse by `Unpack`
- [ ] `ManifestUnpacker` type no longer exists
- [ ] Internal `ApplyLayers` function (in `image/internal/ociutil`) handles decompression and tar extraction with optional filter
- [ ] `DiscoverManifestDescriptors`, `ApplyLayers`, and filter types are internal — not part of the public `image` package API
- [ ] FBCHandler produces identical unpacked output to pre-migration behavior
- [ ] RegistryV1Handler produces identical unpacked output to pre-migration behavior
- [ ] HelmChartHandler produces identical unpacked output to pre-migration behavior
- [ ] HelmChartHandler provenance verification logic is preserved across all verification strategies
- [ ] Progress tracking is a caller concern — no `ProgressRepository` in the library
- [ ] `Unpacker` has no progress or prefetch logic
- [ ] Concurrent fetches into `CachingRepository` are safe via singleflight

## Project Conventions

- [ ] All public types and functions have doc comments
- [ ] No unnecessary public API surface
- [ ] Pure data types with standalone functions — `applyLayer` is an internal function, not a method
- [ ] No legacy dependency usage introduced
- [ ] No cluster dependencies introduced
- [ ] Commit messages follow conventional commits format
- [ ] One logical change per commit
- [ ] Test coverage does not decrease; new code has ≥70% statement coverage
- [ ] `make ci` passes (lint, test, build)
