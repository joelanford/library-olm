package image

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/archive"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/pkg/compression"
	"k8s.io/klog/v2"
)

// FetchImageConfig fetches and decodes the OCI image config from manifest bytes.
// It parses manifestBytes to find the config descriptor, fetches the config blob
// from repo, and decodes it into an [ocispecv1.Image].
//
// Returns an error if manifestBytes is not valid JSON, if the config blob cannot
// be fetched, or if the config blob is not a valid OCI image config.
func FetchImageConfig(ctx context.Context, repo Repository, manifestBytes []byte) (*ocispecv1.Image, error) {
	var manifest ocispecv1.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	reader, err := repo.FetchBlob(ctx, manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("fetching config blob: %w", err)
	}

	cfg, err := decodeImageConfig(reader)
	return cfg, errors.Join(err, reader.Close())
}

func decodeImageConfig(reader io.Reader) (*ocispecv1.Image, error) {
	var cfg ocispecv1.Image
	if err := json.NewDecoder(reader).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	return &cfg, nil
}

// LayerFilter inspects or modifies a [tar.Header] during layer extraction.
// Return true to keep the entry, false to skip it. Filters may also mutate
// the header (e.g. to rewrite paths or change ownership). See [CombineFilters],
// [OnlyPaths], [RewritePath], and [AsCurrentUser] for built-in filters.
type LayerFilter = archive.Filter

// ManifestUnpacker extracts the filesystem content of an OCI image manifest
// by fetching and applying each layer in order. Layers are automatically
// decompressed (gzip, zstd, etc.) before extraction.
type ManifestUnpacker struct {
	// Filter is applied to each tar entry during layer extraction. If nil,
	// all entries are kept. Use [CombineFilters] to compose multiple filters.
	Filter LayerFilter
}

// Unpack parses manifestBytes as an OCI image manifest, then fetches and
// extracts each layer into dest in order. Returns an error if manifestBytes
// is not valid JSON, if any layer blob cannot be fetched, or if layer
// extraction fails.
func (u *ManifestUnpacker) Unpack(ctx context.Context, repo Repository, manifestBytes []byte, dest string) error {
	l := klog.FromContext(ctx)

	var manifest ocispecv1.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	l.V(1).Info("unpacking image", "path", dest)
	for i, layer := range manifest.Layers {
		if err := u.applyLayer(ctx, repo, dest, layer); err != nil {
			return fmt.Errorf("applying layer %d: %w", i, err)
		}
		l.V(1).Info("applied layer", "layer", i)
	}
	return nil
}

func (u *ManifestUnpacker) applyLayer(ctx context.Context, repo Repository, dest string, layer ocispecv1.Descriptor) error {
	reader, err := repo.FetchBlob(ctx, layer)
	if err != nil {
		return err
	}

	err = u.decompressAndApply(ctx, dest, reader)
	return errors.Join(err, reader.Close())
}

func (u *ManifestUnpacker) decompressAndApply(ctx context.Context, dest string, reader io.Reader) error {
	decompressed, _, err := compression.AutoDecompress(reader)
	if err != nil {
		return fmt.Errorf("decompressing layer: %w", err)
	}

	err = u.applyArchive(ctx, dest, decompressed)
	return errors.Join(err, decompressed.Close())
}

func (u *ManifestUnpacker) applyArchive(ctx context.Context, dest string, reader io.Reader) error {
	var opts []archive.ApplyOpt
	if u.Filter != nil {
		opts = append(opts, archive.WithFilter(u.Filter))
	}
	_, err := archive.Apply(ctx, dest, reader, opts...)
	return err
}

// CombineFilters executes each filter in order. If any filter returns false or an error,
// the combined filter immediately returns that result.
func CombineFilters(filters ...LayerFilter) LayerFilter {
	return func(h *tar.Header) (bool, error) {
		for _, filter := range filters {
			keep, err := filter(h)
			if err != nil {
				return false, err
			}
			if !keep {
				return false, nil
			}
		}
		return true, nil
	}
}

// OnlyPaths returns a [LayerFilter] that keeps only tar entries at or under any
// of the given paths, skipping everything else. Leading slashes are stripped from
// both the filter paths and tar entry names before comparison. Empty strings in
// paths are ignored.
func OnlyPaths(paths ...string) LayerFilter {
	wantPaths := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != "" {
			wantPaths = append(wantPaths, path.Clean(strings.TrimPrefix(p, "/")))
		}
	}

	return func(h *tar.Header) (bool, error) {
		headerPath := path.Clean(strings.TrimPrefix(h.Name, "/"))
		for _, wantPath := range wantPaths {
			relPath, err := filepath.Rel(wantPath, headerPath)
			if err != nil {
				return false, fmt.Errorf("getting relative path: %w", err)
			}
			if relPath != ".." && !strings.HasPrefix(relPath, "../") {
				return true, nil
			}
		}
		return false, nil
	}
}

// RewritePath returns a filter that rewrites tar entry paths from srcPath to destPath.
// For example, RewritePath("/configs", "/") rewrites "/configs/foo" to "/foo".
// Entries not under srcPath are kept unmodified.
func RewritePath(srcPath, destPath string) LayerFilter {
	cleanSrc := path.Clean(strings.TrimPrefix(srcPath, "/"))
	cleanDest := path.Clean(strings.TrimPrefix(destPath, "/"))

	return func(h *tar.Header) (bool, error) {
		headerPath := path.Clean(strings.TrimPrefix(h.Name, "/"))

		relPath, err := filepath.Rel(cleanSrc, headerPath)
		if err != nil {
			return true, nil
		}
		if relPath == ".." || strings.HasPrefix(relPath, "../") {
			return true, nil
		}

		h.Name = path.Join(cleanDest, relPath)
		return true, nil
	}
}

// AsCurrentUser rewrites tar entry ownership and permissions so that files
// extracted from container images (typically owned by root with restrictive
// modes) are usable by the current non-root process.
func AsCurrentUser() LayerFilter {
	uid := os.Getuid()
	gid := os.Getgid()
	return func(h *tar.Header) (bool, error) {
		h.Uid = uid
		h.Gid = gid
		if h.Typeflag == tar.TypeDir {
			h.Mode |= 0700
		} else {
			h.Mode |= 0600
		}
		h.PAXRecords = nil
		h.Xattrs = nil //nolint:staticcheck
		return true, nil
	}
}
