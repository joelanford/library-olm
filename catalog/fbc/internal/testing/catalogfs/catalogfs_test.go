package catalogfs_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joelanford/library-olm/catalog/fbc/internal/testing/catalogfs"
)

func TestBuilder_DefaultDerivation(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("my-operator").
		WithChannel("my-operator", "stable",
			catalogfs.Entry("1.0.0"),
		).
		WithBundle("my-operator", "1.0.0").
		Build()

	blobs := parseBlobs(t, fsys)
	require.Len(t, blobs, 3)

	pkg := findBlob(t, blobs, "olm.package", "my-operator")
	assert.Equal(t, "my-operator", pkg["name"])

	ch := findBlob(t, blobs, "olm.channel", "stable")
	assert.Equal(t, "my-operator", ch["package"])
	assert.Equal(t, []any{map[string]any{"name": "my-operator.v1.0.0"}}, ch["entries"])

	bndl := findBlob(t, blobs, "olm.bundle", "my-operator.v1.0.0")
	assert.Equal(t, "my-operator", bndl["package"])
	assert.Equal(t, "quay.io/my-operator/bundle:v1.0.0", bndl["image"])
	assert.Equal(t, []any{map[string]any{
		"type":  "olm.package",
		"value": map[string]any{"packageName": "my-operator", "version": "1.0.0"},
	}}, bndl["properties"])
}

func TestBuilder_Replaces(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("my-op").
		WithChannel("my-op", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("1.1.0", catalogfs.Replaces("1.0.0")),
		).
		WithBundle("my-op", "1.0.0").
		WithBundle("my-op", "1.1.0").
		Build()

	blobs := parseBlobs(t, fsys)
	ch := findBlob(t, blobs, "olm.channel", "stable")
	entries := ch["entries"].([]any)
	require.Len(t, entries, 2)

	entry1 := entries[1].(map[string]any)
	assert.Equal(t, "my-op.v1.1.0", entry1["name"])
	assert.Equal(t, "my-op.v1.0.0", entry1["replaces"])
}

func TestBuilder_Skips(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable",
			catalogfs.Entry("2.0.0", catalogfs.Skips("0.9.0", "1.0.0")),
		).
		WithBundle("op", "2.0.0").
		Build()

	blobs := parseBlobs(t, fsys)
	ch := findBlob(t, blobs, "olm.channel", "stable")
	entries := ch["entries"].([]any)
	entry := entries[0].(map[string]any)
	skips := entry["skips"].([]any)
	assert.Equal(t, []any{"op.v0.9.0", "op.v1.0.0"}, skips)
}

func TestBuilder_SkipRange(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable",
			catalogfs.Entry("2.0.0", catalogfs.SkipRange(">=1.0.0 <2.0.0")),
		).
		WithBundle("op", "2.0.0").
		Build()

	blobs := parseBlobs(t, fsys)
	ch := findBlob(t, blobs, "olm.channel", "stable")
	entries := ch["entries"].([]any)
	entry := entries[0].(map[string]any)
	assert.Equal(t, ">=1.0.0 <2.0.0", entry["skipRange"])
}

func TestBuilder_WithName(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable",
			catalogfs.Entry("1.0.0"),
		).
		WithBundle("op", "1.0.0", catalogfs.WithName("custom-name")).
		Build()

	blobs := parseBlobs(t, fsys)
	bndl := findBlob(t, blobs, "olm.bundle", "custom-name")
	assert.Equal(t, "quay.io/op/bundle:v1.0.0", bndl["image"])
}

func TestBuilder_WithImage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable",
			catalogfs.Entry("1.0.0"),
		).
		WithBundle("op", "1.0.0", catalogfs.WithImage("registry.example.com/op:latest")).
		Build()

	blobs := parseBlobs(t, fsys)
	bndl := findBlob(t, blobs, "olm.bundle", "op.v1.0.0")
	assert.Equal(t, "registry.example.com/op:latest", bndl["image"])
}

func TestBuilder_WithRelease(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable",
			catalogfs.Entry("1.0.0"),
		).
		WithBundle("op", "1.0.0", catalogfs.WithRelease("rc1")).
		Build()

	blobs := parseBlobs(t, fsys)
	bndl := findBlob(t, blobs, "olm.bundle", "op.v1.0.0")
	props := bndl["properties"].([]any)
	prop := props[0].(map[string]any)
	value := prop["value"].(map[string]any)
	assert.Equal(t, "rc1", value["release"])
}

func TestBuilder_MultiplePackages(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("alpha-op").
		WithPackage("beta-op").
		WithChannel("alpha-op", "stable",
			catalogfs.Entry("1.0.0"),
		).
		WithChannel("beta-op", "stable",
			catalogfs.Entry("2.0.0"),
		).
		WithBundle("alpha-op", "1.0.0").
		WithBundle("beta-op", "2.0.0").
		Build()

	blobs := parseBlobs(t, fsys)
	require.Len(t, blobs, 6)

	var pkgNames []string
	for _, b := range blobs {
		if b["schema"] == "olm.package" {
			pkgNames = append(pkgNames, b["name"].(string))
		}
	}
	slices.Sort(pkgNames)
	assert.Equal(t, []string{"alpha-op", "beta-op"}, pkgNames)
}

func TestBuilder_ParseableByWalkMetasFS(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("my-op").
		WithChannel("my-op", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("1.1.0", catalogfs.Replaces("1.0.0")),
		).
		WithBundle("my-op", "1.0.0").
		WithBundle("my-op", "1.1.0").
		Build()

	var mu sync.Mutex
	var schemas []string
	err := declcfg.WalkMetasFS(context.Background(), fsys, func(_ string, meta *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}
		mu.Lock()
		schemas = append(schemas, meta.Schema)
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)
	slices.Sort(schemas)
	assert.Equal(t, []string{"olm.bundle", "olm.bundle", "olm.channel", "olm.package"}, schemas)
}

func TestBuilder_SeparateFiles(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("op", "1.0.0").
		Build()

	require.Len(t, fsys, 3)
	assert.Contains(t, fsys, "op/olm.package.json")
	assert.Contains(t, fsys, "op/olm.channel.stable.json")
	assert.Contains(t, fsys, "op/olm.bundle.op.v1.0.0.json")
}

func TestBuilder_WithCustom(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithCustom("op", "olm.custom.thing", "whatever").
		Build()

	blobs := parseBlobs(t, fsys)
	custom := findBlob(t, blobs, "olm.custom.thing", "whatever")
	assert.Equal(t, "op", custom["package"])
}

func TestBuilder_WithCustomExtraFields(t *testing.T) {
	fsys := catalogfs.Builder().
		WithCustom("op", "olm.bundle", "op.v1.0.0", "properties", "not-an-array").
		Build()

	blobs := parseBlobs(t, fsys)
	bndl := findBlob(t, blobs, "olm.bundle", "op.v1.0.0")
	assert.Equal(t, "op", bndl["package"])
	assert.Equal(t, "not-an-array", bndl["properties"])
}

func TestBuilder_WithCustomNoPackage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithCustom("", "olm.channel", "stable", "entries", "not-an-array").
		Build()

	blobs := parseBlobs(t, fsys)
	ch := findBlob(t, blobs, "olm.channel", "stable")
	_, hasPkg := ch["package"]
	assert.False(t, hasPkg, "empty pkg should omit the package field")
	assert.Equal(t, "not-an-array", ch["entries"])
}

func TestBuilder_WithCustomFilePath(t *testing.T) {
	fsys := catalogfs.Builder().
		WithCustom("my-op", "olm.custom", "thing").
		Build()

	assert.Contains(t, fsys, "my-op/olm.custom.thing.json")
}

func TestBuilder_WithCustomFilePathNoPackage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithCustom("", "olm.channel", "stable").
		Build()

	assert.Contains(t, fsys, "olm.channel.stable.json")
}

func TestBuilder_EmptyBuilder(t *testing.T) {
	fsys := catalogfs.Builder().Build()
	assert.Empty(t, fsys)
}

func TestBuilder_WithEmptyImage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable",
			catalogfs.Entry("1.0.0"),
		).
		WithBundle("op", "1.0.0", catalogfs.WithImage("")).
		Build()

	blobs := parseBlobs(t, fsys)
	bndl := findBlob(t, blobs, "olm.bundle", "op.v1.0.0")
	_, hasImage := bndl["image"]
	assert.False(t, hasImage, "empty image override should omit the image field")
}

func parseBlobs(t *testing.T, fsys fstest.MapFS) []map[string]any {
	t.Helper()
	var blobs []map[string]any
	for path, file := range fsys {
		if file.Mode.IsDir() {
			continue
		}
		var blob map[string]any
		require.NoError(t, json.Unmarshal(file.Data, &blob), "unmarshal %s", path)
		blobs = append(blobs, blob)
	}
	return blobs
}

func findBlob(t *testing.T, blobs []map[string]any, schema, name string) map[string]any {
	t.Helper()
	for _, b := range blobs {
		if b["schema"] == schema && b["name"] == name {
			return b
		}
	}
	t.Fatalf("blob not found: schema=%q name=%q", schema, name)
	return nil
}

// Verify the FS is walkable as a real directory tree.
func TestBuilder_WalkableFS(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("op").
		WithChannel("op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("op", "1.0.0").
		Build()

	var paths []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	require.NoError(t, err)
	slices.Sort(paths)
	assert.Equal(t, []string{
		"op/olm.bundle.op.v1.0.0.json",
		"op/olm.channel.stable.json",
		"op/olm.package.json",
	}, paths)
}
