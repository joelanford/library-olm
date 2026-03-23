package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/operator-framework/library-go/image/internal/testutil"
)

func TestCombineFilters(t *testing.T) {
	passFilter := func(_ *tar.Header) (bool, error) { return true, nil }
	rejectFilter := func(_ *tar.Header) (bool, error) { return false, nil }
	errorFilter := func(_ *tar.Header) (bool, error) { return false, fmt.Errorf("filter error") }

	t.Run("AllPass", func(t *testing.T) {
		combined := CombineFilters(passFilter, passFilter) //nolint:gocritic // intentional duplicate for testing
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.True(t, keep)
	})

	t.Run("FirstRejects", func(t *testing.T) {
		combined := CombineFilters(rejectFilter, passFilter)
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.False(t, keep)
	})

	t.Run("SecondRejects", func(t *testing.T) {
		combined := CombineFilters(passFilter, rejectFilter)
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.False(t, keep)
	})

	t.Run("ErrorPropagated", func(t *testing.T) {
		combined := CombineFilters(passFilter, errorFilter)
		_, err := combined(&tar.Header{Name: "test"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "filter error")
	})

	t.Run("EmptyFilters", func(t *testing.T) {
		combined := CombineFilters()
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.True(t, keep)
	})
}

func TestOnlyPaths(t *testing.T) {
	t.Run("FileUnderWantedPath", func(t *testing.T) {
		filter := OnlyPaths("manifests")
		keep, err := filter(&tar.Header{Name: "manifests/csv.yaml"})
		require.NoError(t, err)
		assert.True(t, keep)
	})

	t.Run("FileNotUnderWantedPath", func(t *testing.T) {
		filter := OnlyPaths("manifests")
		keep, err := filter(&tar.Header{Name: "other/file.yaml"})
		require.NoError(t, err)
		assert.False(t, keep)
	})

	t.Run("LeadingSlashStripped", func(t *testing.T) {
		filter := OnlyPaths("/manifests")
		keep, err := filter(&tar.Header{Name: "manifests/csv.yaml"})
		require.NoError(t, err)
		assert.True(t, keep)
	})
}

func TestRewritePath(t *testing.T) {
	tests := []struct {
		name     string
		srcPath  string
		destPath string
		input    string
		expected string
	}{
		{
			name:     "RewriteUnderSrc",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs/foo",
			expected: "foo",
		},
		{
			name:     "RewriteNestedPath",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs/bar/baz",
			expected: "bar/baz",
		},
		{
			name:     "RewriteSrcDirItself",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs",
			expected: ".",
		},
		{
			name:     "RewriteToDifferentDest",
			srcPath:  "/configs",
			destPath: "/output",
			input:    "configs/foo",
			expected: "output/foo",
		},
		{
			name:     "EntryOutsideSrcUnchanged",
			srcPath:  "/configs",
			destPath: "/",
			input:    "other/file.yaml",
			expected: "other/file.yaml",
		},
		{
			name:     "LeadingSlashOnInput",
			srcPath:  "/configs",
			destPath: "/out",
			input:    "/configs/foo",
			expected: "out/foo",
		},
		{
			name:     "SrcWithoutLeadingSlash",
			srcPath:  "configs",
			destPath: "out",
			input:    "configs/foo",
			expected: "out/foo",
		},
		{
			name:     "SrcPrefixNotDir",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs-extra/foo",
			expected: "configs-extra/foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := RewritePath(tt.srcPath, tt.destPath)
			h := &tar.Header{Name: tt.input}
			keep, err := filter(h)
			require.NoError(t, err)
			assert.True(t, keep)
			assert.Equal(t, tt.expected, h.Name)
		})
	}
}

func TestAsCurrentUser(t *testing.T) {
	filter := AsCurrentUser()

	t.Run("File", func(t *testing.T) {
		h := &tar.Header{
			Name:       "test",
			Typeflag:   tar.TypeReg,
			Uid:        9999,
			Gid:        9999,
			Mode:       0444,
			PAXRecords: map[string]string{"foo": "bar"},
			Xattrs:     map[string]string{"baz": "qux"}, //nolint:staticcheck
		}
		keep, err := filter(h)
		require.NoError(t, err)
		assert.True(t, keep)

		assert.Equal(t, os.Getuid(), h.Uid)
		assert.Equal(t, os.Getgid(), h.Gid)
		assert.Equal(t, int64(0644), h.Mode, "files get rw, not execute")
		assert.Nil(t, h.PAXRecords)
		assert.Nil(t, h.Xattrs) //nolint:staticcheck
	})

	t.Run("Directory", func(t *testing.T) {
		h := &tar.Header{
			Name:     "testdir/",
			Typeflag: tar.TypeDir,
			Uid:      9999,
			Gid:      9999,
			Mode:     0444,
		}
		keep, err := filter(h)
		require.NoError(t, err)
		assert.True(t, keep)

		assert.Equal(t, os.Getuid(), h.Uid)
		assert.Equal(t, os.Getgid(), h.Gid)
		assert.Equal(t, int64(0744), h.Mode, "directories get rwx for traversal")
	})

	t.Run("FilePreservesExistingExecute", func(t *testing.T) {
		h := &tar.Header{
			Name:     "script.sh",
			Typeflag: tar.TypeReg,
			Mode:     0755,
		}
		keep, err := filter(h)
		require.NoError(t, err)
		assert.True(t, keep)
		assert.Equal(t, int64(0755), h.Mode, "existing execute bit should be preserved")
	})

	t.Run("Symlink", func(t *testing.T) {
		h := &tar.Header{
			Name:     "link",
			Typeflag: tar.TypeSymlink,
			Uid:      9999,
			Gid:      9999,
			Mode:     0444,
		}
		keep, err := filter(h)
		require.NoError(t, err)
		assert.True(t, keep)

		assert.Equal(t, os.Getuid(), h.Uid)
		assert.Equal(t, os.Getgid(), h.Gid)
		assert.Equal(t, int64(0644), h.Mode, "symlinks get file mask, not directory mask")
	})
}

func TestOnlyPaths_MultiplePaths(t *testing.T) {
	filter := OnlyPaths("manifests", "metadata")

	keep, err := filter(&tar.Header{Name: "manifests/csv.yaml"})
	require.NoError(t, err)
	assert.True(t, keep)

	keep, err = filter(&tar.Header{Name: "metadata/annotations.yaml"})
	require.NoError(t, err)
	assert.True(t, keep)

	keep, err = filter(&tar.Header{Name: "other/file.yaml"})
	require.NoError(t, err)
	assert.False(t, keep)
}

func TestOnlyPaths_ExactMatch(t *testing.T) {
	filter := OnlyPaths("manifests")
	keep, err := filter(&tar.Header{Name: "manifests"})
	require.NoError(t, err)
	assert.True(t, keep)
}

func TestOnlyPaths_EmptyPathsIgnored(t *testing.T) {
	filter := OnlyPaths("", "manifests", "")
	keep, err := filter(&tar.Header{Name: "manifests/foo"})
	require.NoError(t, err)
	assert.True(t, keep)

	keep, err = filter(&tar.Header{Name: "other/foo"})
	require.NoError(t, err)
	assert.False(t, keep)
}

func TestOnlyPaths_HeaderLeadingSlash(t *testing.T) {
	filter := OnlyPaths("manifests")
	keep, err := filter(&tar.Header{Name: "/manifests/csv.yaml"})
	require.NoError(t, err)
	assert.True(t, keep)
}

func TestOnlyPaths_NoPaths(t *testing.T) {
	filter := OnlyPaths()
	keep, err := filter(&tar.Header{Name: "anything"})
	require.NoError(t, err)
	assert.False(t, keep)
}

func TestOnlyPaths_AllEmptyPaths(t *testing.T) {
	filter := OnlyPaths("", "")
	keep, err := filter(&tar.Header{Name: "anything"})
	require.NoError(t, err)
	assert.False(t, keep)
}

// makeTarGz creates a gzipped tar archive from a map of filename→content.
func makeTarGz(t *testing.T, uid, gid int, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
			Uid:  uid,
			Gid:  gid,
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestFetchImageConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		repo := testutil.NewFakeRepo()

		imgCfg := ocispecv1.Image{
			Author: "test-author",
		}
		cfgBytes := testutil.MustJSON(imgCfg)
		cfgDesc := repo.AddBlob(cfgBytes, "")

		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Config: cfgDesc,
		})

		got, err := FetchImageConfig(ctx, repo, manifestBytes)
		require.NoError(t, err)
		assert.Equal(t, "test-author", got.Author)
	})

	t.Run("InvalidManifestJSON", func(t *testing.T) {
		repo := testutil.NewFakeRepo()
		_, err := FetchImageConfig(ctx, repo, []byte("not json"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing manifest")
	})

	t.Run("BlobFetchError", func(t *testing.T) {
		repo := testutil.NewFakeRepo()
		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Config: ocispecv1.Descriptor{Digest: digest.FromString("missing")},
		})

		_, err := FetchImageConfig(ctx, repo, manifestBytes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetching config blob")
	})

	t.Run("InvalidConfigJSON", func(t *testing.T) {
		repo := testutil.NewFakeRepo()
		cfgDesc := repo.AddBlob([]byte("not json"), "")
		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Config: cfgDesc,
		})

		_, err := FetchImageConfig(ctx, repo, manifestBytes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decoding config")
	})
}

func TestManifestUnpacker_Unpack(t *testing.T) {
	ctx := context.Background()
	uid := os.Getuid()
	gid := os.Getgid()

	t.Run("UnpacksLayersInOrder", func(t *testing.T) {
		repo := testutil.NewFakeRepo()

		layer1 := makeTarGz(t, uid, gid, map[string]string{"file1.txt": "content1"})
		layer2 := makeTarGz(t, uid, gid, map[string]string{"file2.txt": "content2"})
		desc1 := repo.AddBlob(layer1, "")
		desc2 := repo.AddBlob(layer2, "")

		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Layers: []ocispecv1.Descriptor{desc1, desc2},
		})

		dest := t.TempDir()
		u := &ManifestUnpacker{}
		err := u.Unpack(ctx, repo, manifestBytes, dest)
		require.NoError(t, err)

		got1, err := os.ReadFile(filepath.Join(dest, "file1.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content1", string(got1))

		got2, err := os.ReadFile(filepath.Join(dest, "file2.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content2", string(got2))
	})

	t.Run("LaterLayerOverwritesEarlier", func(t *testing.T) {
		repo := testutil.NewFakeRepo()

		layer1 := makeTarGz(t, uid, gid, map[string]string{"file.txt": "old"})
		layer2 := makeTarGz(t, uid, gid, map[string]string{"file.txt": "new"})
		desc1 := repo.AddBlob(layer1, "")
		desc2 := repo.AddBlob(layer2, "")

		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Layers: []ocispecv1.Descriptor{desc1, desc2},
		})

		dest := t.TempDir()
		u := &ManifestUnpacker{}
		err := u.Unpack(ctx, repo, manifestBytes, dest)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dest, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "new", string(got))
	})

	t.Run("NoLayers", func(t *testing.T) {
		repo := testutil.NewFakeRepo()
		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{})

		dest := t.TempDir()
		u := &ManifestUnpacker{}
		err := u.Unpack(ctx, repo, manifestBytes, dest)
		require.NoError(t, err)
	})

	t.Run("InvalidManifestJSON", func(t *testing.T) {
		repo := testutil.NewFakeRepo()
		u := &ManifestUnpacker{}
		err := u.Unpack(ctx, repo, []byte("not json"), t.TempDir())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing manifest")
	})

	t.Run("BlobFetchError", func(t *testing.T) {
		repo := testutil.NewFakeRepo()
		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Layers: []ocispecv1.Descriptor{
				{Digest: digest.FromString("missing")},
			},
		})

		u := &ManifestUnpacker{}
		err := u.Unpack(ctx, repo, manifestBytes, t.TempDir())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "applying layer 0")
	})

	t.Run("BlobReadError", func(t *testing.T) {
		repo := testutil.NewFakeRepo()

		layerDesc := ocispecv1.Descriptor{Digest: digest.FromString("bad-layer")}
		repo.AddErrorBlob(layerDesc, fmt.Errorf("disk read failed"))

		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Layers: []ocispecv1.Descriptor{layerDesc},
		})

		u := &ManifestUnpacker{}
		err := u.Unpack(ctx, repo, manifestBytes, t.TempDir())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "applying layer 0")
		assert.Contains(t, err.Error(), "disk read failed")
	})

	t.Run("WithFilter", func(t *testing.T) {
		repo := testutil.NewFakeRepo()

		layer := makeTarGz(t, uid, gid, map[string]string{
			"keep.txt": "kept",
			"skip.txt": "skipped",
		})
		desc := repo.AddBlob(layer, "")

		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Layers: []ocispecv1.Descriptor{desc},
		})

		dest := t.TempDir()
		u := &ManifestUnpacker{
			Filter: func(h *tar.Header) (bool, error) {
				return h.Name == "keep.txt", nil
			},
		}
		err := u.Unpack(ctx, repo, manifestBytes, dest)
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(dest, "keep.txt"))
		assert.NoError(t, err)

		_, err = os.Stat(filepath.Join(dest, "skip.txt"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("AsCurrentUserWithRootOwnedTar", func(t *testing.T) {
		repo := testutil.NewFakeRepo()

		layer := makeTarGz(t, 0, 0, map[string]string{"root-owned.txt": "hello"})
		desc := repo.AddBlob(layer, "")

		manifestBytes := testutil.MustJSON(ocispecv1.Manifest{
			Layers: []ocispecv1.Descriptor{desc},
		})

		dest := t.TempDir()
		u := &ManifestUnpacker{Filter: AsCurrentUser()}
		err := u.Unpack(ctx, repo, manifestBytes, dest)
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(dest, "root-owned.txt"))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(got))

		info, err := os.Stat(filepath.Join(dest, "root-owned.txt"))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&0600, "owner should have rw")
	})
}

func TestDecodeImageConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		cfg := ocispecv1.Image{Author: "me"}
		data, _ := json.Marshal(cfg)
		got, err := decodeImageConfig(bytes.NewReader(data))
		require.NoError(t, err)
		assert.Equal(t, "me", got.Author)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		_, err := decodeImageConfig(bytes.NewReader([]byte("{")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decoding config")
	})

	t.Run("EmptyReader", func(t *testing.T) {
		_, err := decodeImageConfig(io.LimitReader(bytes.NewReader(nil), 0))
		require.Error(t, err)
	})
}
