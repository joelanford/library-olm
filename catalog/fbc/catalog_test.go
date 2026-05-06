package fbc

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc/internal/testing/catalogfs"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
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
			names := collectBundleIDs(t, composite.ListBundles(ctx))
			slices.Sort(names)
			assert.Equal(t, []string{"my-operator.v1.0.0", "my-operator.v1.1.0", "my-operator.v1.2.0"}, names)
		})

		t.Run("Successors_Package", func(t *testing.T) {
			names := collectBundleIDs(t, composite.Successors(ctx, "my-operator.v1.0.0"))
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
			names := collectBundleIDs(t, stable.ListBundles(ctx))
			slices.Sort(names)
			assert.Equal(t, []string{"my-operator.v1.0.0", "my-operator.v1.1.0"}, names)
		})

		t.Run("Successors_Channel", func(t *testing.T) {
			names := collectBundleIDs(t, stable.Successors(ctx, "my-operator.v1.0.0"))
			assert.Equal(t, []string{"my-operator.v1.1.0"}, names)
		})

		t.Run("Successors_NoSuccessors", func(t *testing.T) {
			names := collectBundleIDs(t, stable.Successors(ctx, "my-operator.v1.1.0"))
			assert.Empty(t, names)
		})
	})

	t.Run("BundleFields", func(t *testing.T) {
		pkg, err := cat.GetPackage(ctx, "my-operator")
		require.NoError(t, err)
		composite := pkg.(catalogv1.CompositeUpdateGraph)
		stable, err := composite.GetGraph(ctx, "stable")
		require.NoError(t, err)

		var found bundlev1.Bundle
		for b, err := range stable.ListBundles(ctx) {
			require.NoError(t, err)
			if b.ID() == "my-operator.v1.0.0" {
				found = b
				break
			}
		}
		require.NotNil(t, found)
		assert.Equal(t, bundlev1.BundleID("my-operator.v1.0.0"), found.ID())
		assert.Equal(t, "my-operator", found.NameVersionRelease().Name)
		assert.Equal(t, "1.0.0", found.NameVersionRelease().Version.String())
		assert.True(t, found.NameVersionRelease().Release.IsEmpty())
		assert.Equal(t, "docker://quay.io/my-operator/bundle:v1.0.0", found.URI())
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

	names := collectBundleIDs(t, ch.Successors(ctx, "skip-operator.v1.0.0"))
	slices.Sort(names)
	// v1.5.0 replaces v1.0.0, and v2.0.0's skipRange includes v1.0.0
	assert.Equal(t, []string{"skip-operator.v1.5.0", "skip-operator.v2.0.0"}, names)

	names = collectBundleIDs(t, ch.Successors(ctx, "skip-operator.v1.5.0"))
	slices.Sort(names)
	assert.Equal(t, []string{"skip-operator.v2.0.0"}, names)
}

func TestFromFS_SkipsWithPhantomBundle(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("skip-op").
		WithChannel("skip-op", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("2.0.0", catalogfs.Skips("0.9.0")),
		).
		WithBundle("skip-op", "1.0.0").
		WithBundle("skip-op", "2.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "skip-op")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)
	ch, err := composite.GetGraph(ctx, "stable")
	require.NoError(t, err)

	names := collectBundleIDs(t, ch.Successors(ctx, "skip-op.v0.9.0"))
	assert.Equal(t, []string{"skip-op.v2.0.0"}, names)
}

func TestFromFS_DanglingReplaces(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("repl-op").
		WithChannel("repl-op", "stable",
			catalogfs.Entry("1.0.0", catalogfs.Replaces("0.1.0")),
		).
		WithBundle("repl-op", "1.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "repl-op")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)
	ch, err := composite.GetGraph(ctx, "stable")
	require.NoError(t, err)

	names := collectBundleIDs(t, ch.Successors(ctx, "repl-op.v0.1.0"))
	assert.Equal(t, []string{"repl-op.v1.0.0"}, names)
}

func TestFromFS_MultiplePackages(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("alpha-op").
		WithPackage("beta-op").
		WithChannel("alpha-op", "stable", catalogfs.Entry("1.0.0")).
		WithChannel("beta-op", "stable", catalogfs.Entry("2.0.0")).
		WithBundle("alpha-op", "1.0.0").
		WithBundle("beta-op", "2.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	var names []string
	for pkg, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		names = append(names, pkg.Name())
	}
	slices.Sort(names)
	assert.Equal(t, []string{"alpha-op", "beta-op"}, names)

	alphaPkg, err := cat.GetPackage(ctx, "alpha-op")
	require.NoError(t, err)
	alphaNames := collectBundleIDs(t, alphaPkg.(catalogv1.CompositeUpdateGraph).ListBundles(ctx))
	assert.Equal(t, []string{"alpha-op.v1.0.0"}, alphaNames)

	betaPkg, err := cat.GetPackage(ctx, "beta-op")
	require.NoError(t, err)
	betaNames := collectBundleIDs(t, betaPkg.(catalogv1.CompositeUpdateGraph).ListBundles(ctx))
	assert.Equal(t, []string{"beta-op.v2.0.0"}, betaNames)
}

func TestFromFS_UnknownSchemasIgnored(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("my-op").
		WithCustom("my-op", "olm.custom.thing", "whatever").
		WithChannel("my-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("my-op", "1.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "my-op")
	require.NoError(t, err)
	assert.Equal(t, "my-op", pkg.Name())
}

func TestFromFS_InvalidSkipRange(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad-op").
		WithChannel("bad-op", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("2.0.0", catalogfs.SkipRange("<=v1.0.0")),
		).
		WithBundle("bad-op", "1.0.0").
		WithBundle("bad-op", "2.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad-op", "skipRange")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_PreReleaseVersion(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("pre-op").
		WithChannel("pre-op", "stable",
			catalogfs.Entry("1.0.0-rc1"),
			catalogfs.Entry("1.0.0", catalogfs.SkipRange(">=0.9.0 <1.0.0")),
		).
		WithBundle("pre-op", "1.0.0-rc1").
		WithBundle("pre-op", "1.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "pre-op")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)
	ch, err := composite.GetGraph(ctx, "stable")
	require.NoError(t, err)

	var rcBundle bundlev1.Bundle
	for b, err := range ch.ListBundles(ctx) {
		require.NoError(t, err)
		if b.ID() == "pre-op.v1.0.0-rc1" {
			rcBundle = b
		}
	}
	require.NotNil(t, rcBundle)
	nvr := rcBundle.NameVersionRelease()
	assert.Equal(t, "1.0.0-rc1", nvr.Version.String())
	assert.True(t, nvr.Release.IsEmpty())

	// skipRange ">=0.9.0 <1.0.0" should match 1.0.0-rc1 (since 1.0.0-rc1 < 1.0.0 in semver)
	names := collectBundleIDs(t, ch.Successors(ctx, "pre-op.v1.0.0-rc1"))
	assert.Equal(t, []string{"pre-op.v1.0.0"}, names)
}

func TestFromFS_BundleWithRelease(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("rel-op").
		WithChannel("rel-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("rel-op", "1.0.0", catalogfs.WithRelease("rc1")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "rel-op")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)

	var found bundlev1.Bundle
	for b, err := range composite.ListBundles(ctx) {
		require.NoError(t, err)
		found = b
	}
	require.NotNil(t, found)
	nvr := found.NameVersionRelease()
	assert.Equal(t, "1.0.0", nvr.Version.String())
	assert.Equal(t, "rc1", nvr.Release.String())
}

func TestFromFS_InvalidBundleRelease(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad-rel").
		WithChannel("bad-rel", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("bad-rel", "1.0.0", catalogfs.WithRelease("rc@1")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad-rel", "release")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_MissingPackageProperty(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("no-prop").
		WithChannel("no-prop", "stable", catalogfs.Entry("1.0.0")).
		WithCustom("no-prop", "olm.bundle", "no-prop.v1.0.0", "properties", []any{}).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "no-prop", "olm.package")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_InvalidBundleVersion(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad-ver").
		WithChannel("bad-ver", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("bad-ver", "not-semver", catalogfs.WithName("bad-ver.v1.0.0")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad-ver", "version")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_SuccessorsUnknownBundle(t *testing.T) {
	fsys := validCatalogFS()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "my-operator")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)
	ch, err := composite.GetGraph(ctx, "stable")
	require.NoError(t, err)

	names := collectBundleIDs(t, ch.Successors(ctx, "nonexistent.v9.9.9"))
	assert.Empty(t, names)
}

func TestFromFS_MissingBundle(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad-operator").
		WithChannel("bad-operator", "stable", catalogfs.Entry("1.0.0")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad-operator", "unknown bundles")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_EmptyFS(t *testing.T) {
	fsys := catalogfs.Builder().Build()
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

func TestFromFS_BundleURI(t *testing.T) {
	fsys := validCatalogFS()
	ctx := context.Background()

	cat, err := FromFS(ctx, fsys)
	require.NoError(t, err)
	defer func() { require.NoError(t, cat.Close()) }()

	pkg, err := cat.GetPackage(ctx, "my-operator")
	require.NoError(t, err)
	composite := pkg.(catalogv1.CompositeUpdateGraph)

	for b, err := range composite.ListBundles(ctx) {
		require.NoError(t, err)
		assert.NotEmpty(t, b.URI(), "URI should be non-empty for bundle %s", b.ID())
		assert.Contains(t, b.URI(), "docker://", "URI should have docker:// scheme for bundle %s", b.ID())
	}
}

func TestFromFS_BundleWithoutImage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("no-img").
		WithChannel("no-img", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("no-img", "1.0.0", catalogfs.WithImage("")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "no-img", "no image")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_BundleWithInvalidImage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad-img").
		WithChannel("bad-img", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("bad-img", "1.0.0", catalogfs.WithImage("INVALID:::ref")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad-img", "parse image")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_BundleWithUntaggedImage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("untag").
		WithChannel("untag", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("untag", "1.0.0", catalogfs.WithImage("quay.io/foo/bar")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "untag", "tagged or canonical")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_BundleWithUnqualifiedImage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("short").
		WithChannel("short", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("short", "1.0.0", catalogfs.WithImage("busybox:latest")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "short", "parse image")
	assertEmptyCatalog(t, ctx, cat)
}

func collectBundleIDs(t *testing.T, seq func(func(bundlev1.Bundle, error) bool)) []string {
	t.Helper()
	var ids []string
	for b, err := range seq {
		require.NoError(t, err)
		ids = append(ids, string(b.ID()))
	}
	return ids
}

func validCatalogFS() fstest.MapFS {
	return catalogfs.Builder().
		WithPackage("my-operator").
		WithChannel("my-operator", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("1.1.0", catalogfs.Replaces("1.0.0")),
		).
		WithChannel("my-operator", "fast",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("1.2.0", catalogfs.Replaces("1.0.0")),
		).
		WithBundle("my-operator", "1.0.0").
		WithBundle("my-operator", "1.1.0").
		WithBundle("my-operator", "1.2.0").
		Build()
}

func skipRangeCatalogFS() fstest.MapFS {
	return catalogfs.Builder().
		WithPackage("skip-operator").
		WithChannel("skip-operator", "stable",
			catalogfs.Entry("1.0.0"),
			catalogfs.Entry("1.5.0", catalogfs.Replaces("1.0.0")),
			catalogfs.Entry("2.0.0", catalogfs.Replaces("1.5.0"), catalogfs.SkipRange(">=1.0.0 <2.0.0")),
		).
		WithBundle("skip-operator", "1.0.0").
		WithBundle("skip-operator", "1.5.0").
		WithBundle("skip-operator", "2.0.0").
		Build()
}

func requirePackageError(t *testing.T, err error, pkg string, msgSubstring string) {
	t.Helper()
	var pkgErr *PackageError
	found := false
	for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
		if errors.As(e, &pkgErr) && pkgErr.Package == pkg {
			found = true
			assert.Contains(t, pkgErr.Error(), msgSubstring, "PackageError for %q should contain %q", pkg, msgSubstring)
		}
	}
	require.True(t, found, "expected PackageError for package %q, got: %v", pkg, err)
}

func assertEmptyCatalog(t *testing.T, ctx context.Context, cat catalogv1.Catalog) {
	t.Helper()
	var count int
	for _, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		count++
	}
	assert.Equal(t, 0, count)
}

func TestFromFS_MixedValidAndMalformed(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("good-op").
		WithPackage("bad-op").
		WithChannel("good-op", "stable", catalogfs.Entry("1.0.0")).
		WithChannel("bad-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("good-op", "1.0.0").
		WithBundle("bad-op", "not-semver", catalogfs.WithName("bad-op.v1.0.0")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad-op", "version")

	var names []string
	for pkg, err := range cat.ListPackages(ctx) {
		require.NoError(t, err)
		names = append(names, pkg.Name())
	}
	assert.Equal(t, []string{"good-op"}, names)

	_, err = cat.GetPackage(ctx, "bad-op")
	require.Error(t, err)
}

func TestFromFS_AllMalformed(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad1").
		WithPackage("bad2").
		WithChannel("bad1", "stable", catalogfs.Entry("1.0.0")).
		WithChannel("bad2", "stable", catalogfs.Entry("1.0.0", catalogfs.SkipRange("<=bad"))).
		WithBundle("bad1", "not-semver", catalogfs.WithName("bad1.v1.0.0")).
		WithBundle("bad2", "1.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "bad1", "version")
	requirePackageError(t, err, "bad2", "skipRange")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_MalformedPackageBlob(t *testing.T) {
	fsys := catalogfs.Builder().
		WithCustom("", "olm.package", "bad-pkg", "icon", "not-an-object").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.Nil(t, cat)
	assert.Contains(t, err.Error(), "parse package")
}

func TestFromFS_MalformedChannelBlob(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("ch-op").
		WithCustom("ch-op", "olm.channel", "stable", "entries", "not-an-array").
		WithBundle("ch-op", "1.0.0").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "ch-op", "parse channel")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_MalformedBundleBlob(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("b-op").
		WithChannel("b-op", "stable", catalogfs.Entry("1.0.0")).
		WithCustom("b-op", "olm.bundle", "b-op.v1.0.0", "properties", "not-an-array").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	requirePackageError(t, err, "b-op", "parse bundle")
	assertEmptyCatalog(t, ctx, cat)
}

func TestFromFS_MalformedBlobEmptyPackage(t *testing.T) {
	fsys := catalogfs.Builder().
		WithCustom("", "olm.channel", "stable", "entries", "not-an-array").
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.Nil(t, cat)
	assert.Contains(t, err.Error(), "parse channel")
}

func TestFromFS_PackageErrorUnwrap(t *testing.T) {
	fsys := catalogfs.Builder().
		WithPackage("bad-op").
		WithChannel("bad-op", "stable", catalogfs.Entry("1.0.0")).
		WithBundle("bad-op", "not-semver", catalogfs.WithName("bad-op.v1.0.0")).
		Build()
	ctx := context.Background()
	cat, err := FromFS(ctx, fsys)
	require.Error(t, err)
	require.NotNil(t, cat)
	defer func() { require.NoError(t, cat.Close()) }()

	var pkgErr *PackageError
	require.True(t, errors.As(err, &pkgErr))
	assert.Equal(t, "bad-op", pkgErr.Package)
	assert.NotEmpty(t, pkgErr.Errs)
}
