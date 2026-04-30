package fbc

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/blang/semver/v4"
	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromFS_ValidCatalog(t *testing.T) {
	fsys := validCatalogFS()
	ctx := context.Background()

	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	t.Run("ListPackages", func(t *testing.T) {
		var names []string
		for pkg, err := range cat.ListPackages(ctx) {
			require.NoError(t, err)
			names = append(names, pkg.Name())
		}
		assert.Equal(t, []string{"my-operator"}, names)
	})

	t.Run("GetPackage", func(t *testing.T) {
		pkg, err := cat.GetPackage(ctx, "my-operator")
		require.NoError(t, err)
		assert.Equal(t, "my-operator", pkg.Name())
	})

	t.Run("GetPackage_NotFound", func(t *testing.T) {
		_, err := cat.GetPackage(ctx, "nonexistent")
		require.Error(t, err)
	})

	t.Run("CompositeUpdateGraph", func(t *testing.T) {
		pkg, err := cat.GetPackage(ctx, "my-operator")
		require.NoError(t, err)

		composite, ok := pkg.(catalogv1.CompositeUpdateGraph)
		require.True(t, ok, "expected CompositeUpdateGraph")

		t.Run("ListGraphs", func(t *testing.T) {
			var names []string
			for g, err := range composite.ListGraphs(ctx) {
				require.NoError(t, err)
				names = append(names, g.Name())
			}
			slices.Sort(names)
			assert.Equal(t, []string{"fast", "stable"}, names)
		})

		t.Run("GetGraph", func(t *testing.T) {
			g, err := composite.GetGraph(ctx, "stable")
			require.NoError(t, err)
			assert.Equal(t, "stable", g.Name())
		})

		t.Run("GetGraph_NotFound", func(t *testing.T) {
			_, err := composite.GetGraph(ctx, "nonexistent")
			require.Error(t, err)
		})

		t.Run("ListBundles_Package", func(t *testing.T) {
			names := collectBundleNames(t, composite.ListBundles(ctx))
			slices.Sort(names)
			assert.Equal(t, []string{"my-operator.v1.0.0", "my-operator.v1.1.0", "my-operator.v1.2.0"}, names)
		})

		t.Run("Successors_Package", func(t *testing.T) {
			from := bundlev1.NameVersionRelease{
				BundleName: "my-operator.v1.0.0",
				Version:    semver.MustParse("1.0.0"),
			}
			names := collectBundleNames(t, composite.Successors(ctx, from))
			slices.Sort(names)
			assert.Equal(t, []string{"my-operator.v1.1.0", "my-operator.v1.2.0"}, names)
		})
	})

	t.Run("ChannelUpdateGraph", func(t *testing.T) {
		pkg, err := cat.GetPackage(ctx, "my-operator")
		require.NoError(t, err)
		composite := pkg.(catalogv1.CompositeUpdateGraph)

		stable, err := composite.GetGraph(ctx, "stable")
		require.NoError(t, err)

		t.Run("ListBundles_Channel", func(t *testing.T) {
			names := collectBundleNames(t, stable.ListBundles(ctx))
			slices.Sort(names)
			assert.Equal(t, []string{"my-operator.v1.0.0", "my-operator.v1.1.0"}, names)
		})

		t.Run("Successors_Channel", func(t *testing.T) {
			from := bundlev1.NameVersionRelease{
				BundleName: "my-operator.v1.0.0",
				Version:    semver.MustParse("1.0.0"),
			}
			names := collectBundleNames(t, stable.Successors(ctx, from))
			assert.Equal(t, []string{"my-operator.v1.1.0"}, names)
		})

		t.Run("Successors_NoSuccessors", func(t *testing.T) {
			from := bundlev1.NameVersionRelease{
				BundleName: "my-operator.v1.1.0",
				Version:    semver.MustParse("1.1.0"),
			}
			names := collectBundleNames(t, stable.Successors(ctx, from))
			assert.Empty(t, names)
		})
	})
}

func TestFromFS_SkipRange(t *testing.T) {
	fsys := skipRangeCatalogFS()
	ctx := context.Background()

	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "skip-operator")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)
	ch, err := composite.GetGraph(ctx, "stable")
	require.NoError(t, err)

	from100 := bundlev1.NameVersionRelease{BundleName: "skip-operator.v1.0.0", Version: semver.MustParse("1.0.0")}
	names := collectBundleNames(t, ch.Successors(ctx, from100))
	slices.Sort(names)
	// v1.5.0 replaces v1.0.0, and v2.0.0's skipRange includes v1.0.0
	assert.Equal(t, []string{"skip-operator.v1.5.0", "skip-operator.v2.0.0"}, names)

	from150 := bundlev1.NameVersionRelease{BundleName: "skip-operator.v1.5.0", Version: semver.MustParse("1.5.0")}
	names = collectBundleNames(t, ch.Successors(ctx, from150))
	slices.Sort(names)
	assert.Equal(t, []string{"skip-operator.v2.0.0"}, names)
}

func TestFromFS_MissingBundle(t *testing.T) {
	fsys := fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			fbcPackage("bad-operator") +
				fbcChannel("bad-operator", "stable", `[{"name":"missing.v1.0.0"}]`) +
				"",
		)},
	}
	ctx := context.Background()
	_, err := FromFS(ctx, fsys)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown bundles")
}

func TestFromFS_EmptyFS(t *testing.T) {
	fsys := fstest.MapFS{}
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	var count int
	for _, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		count++
	}
	assert.Equal(t, 0, count)
}

func TestClose(t *testing.T) {
	fsys := validCatalogFS()
	ctx := context.Background()

	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	require.NoError(t, cat.Close())
}

func collectBundleNames(t *testing.T, seq func(func(bundlev1.Bundle, error) bool)) []string {
	t.Helper()
	var names []string
	for b, err := range seq {
		require.NoError(t, err)
		names = append(names, b.Name())
	}
	return names
}

func validCatalogFS() fstest.MapFS {
	return fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			fbcPackage("my-operator") +
				fbcChannel("my-operator", "stable", `[{"name":"my-operator.v1.0.0"},{"name":"my-operator.v1.1.0","replaces":"my-operator.v1.0.0"}]`) +
				fbcChannel("my-operator", "fast", `[{"name":"my-operator.v1.0.0"},{"name":"my-operator.v1.2.0","replaces":"my-operator.v1.0.0"}]`) +
				fbcBundle("my-operator", "my-operator.v1.0.0", "1.0.0") +
				fbcBundle("my-operator", "my-operator.v1.1.0", "1.1.0") +
				fbcBundle("my-operator", "my-operator.v1.2.0", "1.2.0"),
		)},
	}
}

func skipRangeCatalogFS() fstest.MapFS {
	return fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			fbcPackage("skip-operator") +
				fbcChannel("skip-operator", "stable",
					`[{"name":"skip-operator.v1.0.0"},{"name":"skip-operator.v1.5.0","replaces":"skip-operator.v1.0.0"},{"name":"skip-operator.v2.0.0","replaces":"skip-operator.v1.5.0","skipRange":">=1.0.0 <2.0.0"}]`) +
				fbcBundle("skip-operator", "skip-operator.v1.0.0", "1.0.0") +
				fbcBundle("skip-operator", "skip-operator.v1.5.0", "1.5.0") +
				fbcBundle("skip-operator", "skip-operator.v2.0.0", "2.0.0"),
		)},
	}
}

func fbcPackage(name string) string {
	return fmt.Sprintf("{\"schema\":\"olm.package\",\"name\":\"%s\"}\n", name)
}

func fbcChannel(pkg, name, entries string) string {
	return fmt.Sprintf("{\"schema\":\"olm.channel\",\"package\":\"%s\",\"name\":\"%s\",\"entries\":%s}\n", pkg, name, entries)
}

func fbcBundle(pkg, name, version string) string {
	return fmt.Sprintf("{\"schema\":\"olm.bundle\",\"package\":\"%s\",\"name\":\"%s\",\"properties\":[{\"type\":\"olm.package\",\"value\":{\"packageName\":\"%s\",\"version\":\"%s\"}}]}\n",
		pkg, name, pkg, version)
}
