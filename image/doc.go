// Package image provides OCI registry access, content caching, and layer
// extraction for container images. It defines the [Repository] interface for
// fetching manifests and blobs, a [CachingRepository] wrapper that deduplicates
// and caches fetched content, and an [Unpacker] that delegates to registered
// [Handler] implementations for format-specific unpacking.
package image
