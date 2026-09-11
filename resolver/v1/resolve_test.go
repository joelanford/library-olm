package resolverv1_test

import (
	"context"
	"errors"
	"testing"

	mmsemver "github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	testutil "github.com/joelanford/library-olm/internal/util/test"
	resolverv1 "github.com/joelanford/library-olm/resolver/v1"
)

func collectBundleIDs(t *testing.T, bundles []bundlev1.Bundle) []string {
	t.Helper()
	var ids []string
	for _, b := range bundles {
		ids = append(ids, string(b.ID()))
	}
	return ids
}

func fakePackage(name string) catalogv1.Package {
	return &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: name}}
}

func fakeCatalog(pkg catalogv1.Package) *testutil.Catalog {
	return &testutil.Catalog{
		CatalogName: "cat",
		Packages:    map[string]catalogv1.Package{"pkg": pkg},
	}
}

func fakeReader(pkg catalogv1.Package) *testutil.StoreReader {
	return &testutil.StoreReader{Catalogs: []catalogv1.Catalog{fakeCatalog(pkg)}}
}

func TestResolve_ChannelFiltering(t *testing.T) {
	bundle := func(version string) bundlev1.Bundle {
		return testutil.NewBundle(t, "pkg", version, "")
	}

	tests := []struct {
		name     string
		paths    [][]string
		expected []string
		listed   string
	}{
		{name: "omitted paths use package root", expected: []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"}, listed: "pkg"},
		{name: "empty path addresses package root", paths: [][]string{{}}, expected: []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"}, listed: "pkg"},
		{name: "top-level channel", paths: [][]string{{"stable"}}, expected: []string{"pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"}, listed: "stable"},
		{name: "nested channel", paths: [][]string{{"stable", "lts"}}, expected: []string{"pkg.v1.0.0"}, listed: "lts"},
		{name: "multiple paths form union", paths: [][]string{{"stable", "lts"}, {"fast"}}, expected: []string{"pkg.v4.0.0", "pkg.v1.0.0"}},
		{name: "missing top-level path is omitted", paths: [][]string{{"missing"}}},
		{name: "missing nested path is omitted", paths: [][]string{{"stable", "missing"}}},
		{name: "path cannot continue through leaf", paths: [][]string{{"fast", "nested"}}},
		{name: "missing path does not hide valid path", paths: [][]string{{"missing"}, {"fast"}}, expected: []string{"pkg.v4.0.0"}, listed: "fast"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{
				bundle("1.0.0"), bundle("2.0.0"), bundle("3.0.0"), bundle("4.0.0"),
			}}
			lts := &testutil.LeafGraph{GraphName: "lts", Bundles: []bundlev1.Bundle{bundle("1.0.0")}}
			stable := &testutil.CompositeGraph{
				LeafGraph: &testutil.LeafGraph{GraphName: "stable", Bundles: []bundlev1.Bundle{
					bundle("1.0.0"), bundle("2.0.0"), bundle("3.0.0"),
				}},
				Graphs: map[string]catalogv1.UpdateGraph{"lts": lts},
			}
			fast := &testutil.LeafGraph{GraphName: "fast", Bundles: []bundlev1.Bundle{bundle("4.0.0")}}
			pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
				LeafGraph: root,
				Graphs: map[string]catalogv1.UpdateGraph{
					"stable": stable,
					"fast":   fast,
				},
			}}

			var opts []resolverv1.ResolveOption
			if tt.paths != nil {
				opts = append(opts, resolverv1.WithGraphs(tt.paths))
			}
			result, err := resolverv1.Resolve(context.Background(), fakeReader(pkg), "pkg", opts...)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected, collectBundleIDs(t, result.Bundles))
			calls := map[string]int{
				"pkg":    root.ListBundlesCalls,
				"stable": stable.ListBundlesCalls,
				"lts":    lts.ListBundlesCalls,
				"fast":   fast.ListBundlesCalls,
			}
			if tt.name == "multiple paths form union" {
				assert.Equal(t, map[string]int{"pkg": 0, "stable": 0, "lts": 1, "fast": 1}, calls)
			} else if tt.listed != "" {
				expectedCalls := map[string]int{"pkg": 0, "stable": 0, "lts": 0, "fast": 0}
				expectedCalls[tt.listed] = 1
				assert.Equal(t, expectedCalls, calls)
			} else {
				assert.Equal(t, map[string]int{"pkg": 0, "stable": 0, "lts": 0, "fast": 0}, calls)
			}
		})
	}
}

func TestResolve_BundleSourceDelegation(t *testing.T) {
	from := testutil.NewBundleIdentity(t, "pkg", "1.0.0", "")
	immediateA := testutil.NewBundle(t, "pkg", "2.0.0", "")
	immediateB := testutil.NewBundle(t, "pkg", "4.0.0", "")
	transitive := testutil.NewBundle(t, "pkg", "3.0.0", "")

	t.Run("omitted WithSuccessorsOf uses ListBundles", func(t *testing.T) {
		graph := &testutil.LeafGraph{
			GraphName:        "pkg",
			Bundles:          []bundlev1.Bundle{transitive, immediateA, immediateB},
			SuccessorBundles: []bundlev1.Bundle{immediateA, immediateB},
		}
		result, err := resolverv1.Resolve(context.Background(), fakeReader(&testutil.Package{LeafGraph: graph}), "pkg")

		require.NoError(t, err)
		assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))
		assert.Equal(t, 1, graph.ListBundlesCalls)
		assert.Empty(t, graph.SuccessorsCalls)
	})

	t.Run("WithSuccessorsOf delegates immediate-only input to graph", func(t *testing.T) {
		graph := &testutil.LeafGraph{
			GraphName:        "pkg",
			Bundles:          []bundlev1.Bundle{transitive, immediateA, immediateB},
			SuccessorBundles: []bundlev1.Bundle{immediateA, immediateB},
		}
		result, err := resolverv1.Resolve(
			context.Background(),
			fakeReader(&testutil.Package{LeafGraph: graph}),
			"pkg",
			resolverv1.WithSuccessorsOf(from),
		)

		require.NoError(t, err)
		assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))
		assert.Zero(t, graph.ListBundlesCalls)
		require.Len(t, graph.SuccessorsCalls, 1)
		assert.Equal(t, from, graph.SuccessorsCalls[0])
	})
}

func TestResolve_VersionConstraint(t *testing.T) {
	graph := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{
		testutil.NewBundle(t, "pkg", "2.0.0", ""),
		testutil.NewBundle(t, "pkg", "1.5.0", ""),
		testutil.NewBundle(t, "pkg", "1.0.0", ""),
		testutil.NewBundle(t, "pkg", "1.0.0-beta.1", ""),
		testutil.NewBundle(t, "pkg", "0.9.0", ""),
	}}
	constraint, err := mmsemver.NewConstraint(">=1.0.0, <2.0.0")
	require.NoError(t, err)

	result, err := resolverv1.Resolve(
		context.Background(),
		fakeReader(&testutil.Package{LeafGraph: graph}),
		"pkg",
		resolverv1.WithMastermindsVersionConstraint(*constraint),
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v1.5.0", "pkg.v1.0.0"}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_IteratorErrors(t *testing.T) {
	listErr := errors.New("list bundles failed")
	successorsErr := errors.New("successors failed")
	from := testutil.NewBundleIdentity(t, "pkg", "1.0.0", "")

	tests := []struct {
		name  string
		graph *testutil.LeafGraph
		opts  []resolverv1.ResolveOption
		err   error
	}{
		{
			name: "ListBundles error after a value",
			graph: &testutil.LeafGraph{
				GraphName:      "pkg",
				Bundles:        []bundlev1.Bundle{testutil.NewBundle(t, "pkg", "2.0.0", "")},
				ListBundlesErr: listErr,
			},
			err: listErr,
		},
		{
			name: "Successors error after a value",
			graph: &testutil.LeafGraph{
				GraphName:        "pkg",
				SuccessorBundles: []bundlev1.Bundle{testutil.NewBundle(t, "pkg", "2.0.0", "")},
				SuccessorsErr:    successorsErr,
			},
			opts: []resolverv1.ResolveOption{resolverv1.WithSuccessorsOf(from)},
			err:  successorsErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolverv1.Resolve(
				context.Background(),
				fakeReader(&testutil.Package{LeafGraph: tt.graph}),
				"pkg",
				tt.opts...,
			)

			assert.Nil(t, result)
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestResolve_DeduplicatesByBundleID(t *testing.T) {
	first := testutil.NewBundle(t, "pkg", "1.0.0", "")
	first.BundleIdentity = testutil.BundleIdentity{
		BundleID: "same-id",
		NVR:      first.NameVersionRelease(),
	}
	duplicate := testutil.NewBundle(t, "pkg", "9.0.0", "")
	duplicate.BundleIdentity = testutil.BundleIdentity{
		BundleID: "same-id",
		NVR:      duplicate.NameVersionRelease(),
	}
	unique := testutil.NewBundle(t, "pkg", "2.0.0", "")
	left := &testutil.LeafGraph{GraphName: "left", Bundles: []bundlev1.Bundle{first, first}}
	right := &testutil.LeafGraph{GraphName: "right", Bundles: []bundlev1.Bundle{duplicate, unique}}
	pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "pkg"},
		Graphs: map[string]catalogv1.UpdateGraph{
			"left":  left,
			"right": right,
		},
	}}

	result, err := resolverv1.Resolve(
		context.Background(),
		fakeReader(pkg),
		"pkg",
		resolverv1.WithGraphs([][]string{{"left"}, {"right"}}),
	)

	require.NoError(t, err)
	require.Len(t, result.Bundles, 2)
	assert.Same(t, unique, result.Bundles[0])
	assert.Same(t, first, result.Bundles[1], "the first bundle seen for an ID is retained")
}

func TestResolve_CandidateOrdering(t *testing.T) {
	equalZ := testutil.NewBundle(t, "pkg", "2.0.0", "")
	equalZ.BundleIdentity = testutil.BundleIdentity{BundleID: "z", NVR: equalZ.NameVersionRelease()}
	equalA := testutil.NewBundle(t, "pkg", "2.0.0", "")
	equalA.BundleIdentity = testutil.BundleIdentity{BundleID: "a", NVR: equalA.NameVersionRelease()}
	graph := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{
		equalZ,
		testutil.NewBundle(t, "pkg", "1.0.0", ""),
		testutil.NewBundle(t, "pkg", "2.0.0", "rc.2"),
		testutil.NewBundle(t, "pkg", "3.0.0", ""),
		testutil.NewBundle(t, "pkg", "2.0.0", "rc.10"),
		equalA,
	}}

	result, err := resolverv1.Resolve(context.Background(), fakeReader(&testutil.Package{LeafGraph: graph}), "pkg")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"pkg.v3.0.0",
		"pkg.v2.0.0-rc.10",
		"pkg.v2.0.0-rc.2",
		"a",
		"z",
		"pkg.v1.0.0",
	}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_PreferNonDeprecatedFakeBundles(t *testing.T) {
	deprecatedV3 := &testutil.DeprecatedBundle{Bundle: testutil.NewBundle(t, "pkg", "3.0.0", ""), Message: "deprecated"}
	deprecatedV1 := &testutil.DeprecatedBundle{Bundle: testutil.NewBundle(t, "pkg", "1.0.0", "")}
	graph := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{
		deprecatedV1,
		testutil.NewBundle(t, "pkg", "2.0.0", ""),
		deprecatedV3,
		testutil.NewBundle(t, "pkg", "0.5.0", ""),
	}}

	result, err := resolverv1.Resolve(
		context.Background(),
		fakeReader(&testutil.Package{LeafGraph: graph}),
		"pkg",
		resolverv1.PreferNonDeprecatedBundles(),
	)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"pkg.v2.0.0",
		"pkg.v0.5.0",
		"pkg.v3.0.0",
		"pkg.v1.0.0",
	}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_PackagePriorityGroupReads(t *testing.T) {
	t.Run("catalog list error is propagated before package reads", func(t *testing.T) {
		listErr := errors.New("catalog list failed")
		cat := &testutil.Catalog{
			CatalogName: "cat",
			Packages:    map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{cat},
			ListErr:  listErr,
		}, "pkg")

		assert.Nil(t, result)
		require.ErrorIs(t, err, listErr)
		assert.Empty(t, cat.GetPackageCalls)
	})

	t.Run("absence in every priority group returns no result", func(t *testing.T) {
		high := &testutil.Catalog{CatalogName: "high", CatalogPriority: 10}
		low := &testutil.Catalog{CatalogName: "low", CatalogPriority: 5}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{low, high},
		}, "pkg")

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, []string{"pkg"}, high.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, low.GetPackageCalls)
	})

	t.Run("current group errors are all propagated after full group check", func(t *testing.T) {
		firstErr := errors.New("first read failed")
		secondErr := errors.New("second read failed")
		match := &testutil.Catalog{
			CatalogName:     "match",
			CatalogPriority: 10,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}
		failedA := &testutil.Catalog{CatalogName: "failed-a", CatalogPriority: 10, GetPackageErr: firstErr}
		failedB := &testutil.Catalog{CatalogName: "failed-b", CatalogPriority: 10, GetPackageErr: secondErr}
		lower := &testutil.Catalog{
			CatalogName:     "lower",
			CatalogPriority: 1,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{lower, failedB, match, failedA},
		}, "pkg")

		assert.Nil(t, result)
		require.ErrorIs(t, err, firstErr)
		require.ErrorIs(t, err, secondErr)
		assert.Equal(t, []string{"pkg"}, match.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, failedA.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, failedB.GetPackageCalls)
		assert.Empty(t, lower.GetPackageCalls)
	})

	t.Run("only typed absence permits fallthrough", func(t *testing.T) {
		highA := &testutil.Catalog{CatalogName: "high-a", CatalogPriority: 10}
		highB := &testutil.Catalog{CatalogName: "high-b", CatalogPriority: 10}
		low := &testutil.Catalog{
			CatalogName:     "low",
			CatalogPriority: 5,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{low, highA, highB},
		}, "pkg")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "low", result.Catalog.Name())
		assert.Equal(t, []string{"pkg"}, highA.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, highB.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, low.GetPackageCalls)
	})

	t.Run("untyped not found text is a read error", func(t *testing.T) {
		untyped := errors.New("package not found")
		high := &testutil.Catalog{CatalogName: "high", CatalogPriority: 10, GetPackageErr: untyped}
		low := &testutil.Catalog{
			CatalogName:     "low",
			CatalogPriority: 5,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{low, high},
		}, "pkg")

		assert.Nil(t, result)
		require.ErrorIs(t, err, untyped)
		assert.Empty(t, low.GetPackageCalls)
	})

	t.Run("unique match checks its full group and avoids lower priority reads", func(t *testing.T) {
		match := &testutil.Catalog{
			CatalogName:     "match",
			CatalogPriority: 10,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}
		absent := &testutil.Catalog{CatalogName: "absent", CatalogPriority: 10}
		lower := &testutil.Catalog{
			CatalogName:     "lower",
			CatalogPriority: 1,
			GetPackageErr:   errors.New("must not be read"),
		}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{lower, match, absent},
		}, "pkg")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "match", result.Catalog.Name())
		assert.Equal(t, []string{"pkg"}, match.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, absent.GetPackageCalls)
		assert.Empty(t, lower.GetPackageCalls)
	})

	t.Run("ambiguity is typed and checks every catalog in the group", func(t *testing.T) {
		catB := &testutil.Catalog{
			CatalogName:     "cat-b",
			CatalogPriority: 7,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}
		absent := &testutil.Catalog{CatalogName: "absent", CatalogPriority: 7}
		catA := &testutil.Catalog{
			CatalogName:     "cat-a",
			CatalogPriority: 7,
			Packages:        map[string]catalogv1.Package{"pkg": fakePackage("pkg")},
		}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{catB, absent, catA},
		}, "pkg")

		assert.Nil(t, result)
		var ambiguity *resolverv1.AmbiguousPackageError
		require.ErrorAs(t, err, &ambiguity)
		assert.Equal(t, "pkg", ambiguity.Package)
		assert.Equal(t, 7, ambiguity.Priority)
		assert.Equal(t, []string{"cat-a", "cat-b"}, ambiguity.Catalogs)
		assert.Equal(t, []string{"pkg"}, catA.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, catB.GetPackageCalls)
		assert.Equal(t, []string{"pkg"}, absent.GetPackageCalls)
	})
}

func TestResolve_GraphLookupErrors(t *testing.T) {
	t.Run("typed absence omits the path", func(t *testing.T) {
		pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
			LeafGraph: &testutil.LeafGraph{GraphName: "pkg"},
		}}
		cat := &testutil.Catalog{CatalogName: "cat", Packages: map[string]catalogv1.Package{"pkg": pkg}}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{cat},
		}, "pkg", resolverv1.WithGraphs([][]string{{"missing"}}))

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Bundles)
		assert.Equal(t, []string{"missing"}, pkg.GetGraphCalls)
	})

	t.Run("real lookup failure is propagated", func(t *testing.T) {
		readErr := errors.New("graph read failed")
		pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
			LeafGraph:   &testutil.LeafGraph{GraphName: "pkg"},
			GetGraphErr: readErr,
		}}
		cat := &testutil.Catalog{CatalogName: "cat", Packages: map[string]catalogv1.Package{"pkg": pkg}}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{cat},
		}, "pkg", resolverv1.WithGraphs([][]string{{"stable"}}))

		assert.Nil(t, result)
		require.ErrorIs(t, err, readErr)
	})

	t.Run("nested lookup failure is propagated", func(t *testing.T) {
		readErr := errors.New("nested graph read failed")
		stable := &testutil.CompositeGraph{
			LeafGraph:   &testutil.LeafGraph{GraphName: "stable"},
			GetGraphErr: readErr,
		}
		pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
			LeafGraph: &testutil.LeafGraph{GraphName: "pkg"},
			Graphs:    map[string]catalogv1.UpdateGraph{"stable": stable},
		}}
		cat := &testutil.Catalog{CatalogName: "cat", Packages: map[string]catalogv1.Package{"pkg": pkg}}

		result, err := resolverv1.Resolve(context.Background(), &testutil.StoreReader{
			Catalogs: []catalogv1.Catalog{cat},
		}, "pkg", resolverv1.WithGraphs([][]string{{"stable", "lts"}}))

		assert.Nil(t, result)
		require.ErrorIs(t, err, readErr)
		assert.Equal(t, []string{"stable"}, pkg.GetGraphCalls)
		assert.Equal(t, []string{"lts"}, stable.GetGraphCalls)
	})
}
