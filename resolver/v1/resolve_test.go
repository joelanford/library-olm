package resolverv1_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	mmsemver "github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
	testutil "github.com/joelanford/library-olm/internal/util/test"
	resolverv1 "github.com/joelanford/library-olm/resolver/v1"
)

// --- Graph DSL ---

type graphOption struct {
	name     string
	bundles  []string
	edges    []edge
	ranges   []predRange
	children []graphOption
}

type edge struct{ from, to string }
type predRange struct{ bundle, versionRange string }

type graphCfg func(*graphOption)

func withEdge(from, to string) graphCfg {
	return func(g *graphOption) { g.edges = append(g.edges, edge{from, to}) }
}

func withRange(bundle, versionRange string) graphCfg {
	return func(g *graphOption) { g.ranges = append(g.ranges, predRange{bundle, versionRange}) }
}

func withChild(child graphOption) graphCfg {
	return func(g *graphOption) { g.children = append(g.children, child) }
}

func subGraph(name string, bundles []string, opts ...graphCfg) graphOption {
	g := graphOption{name: name, bundles: bundles}
	for _, opt := range opts {
		opt(&g)
	}
	return g
}

func graph(pkg string, bundles []string, subGraphs ...graphOption) catalogv1.Importer {
	return &graphImporter{pkg: pkg, bundles: bundles, subGraphs: subGraphs}
}

type graphImporter struct {
	pkg       string
	bundles   []string
	subGraphs []graphOption
}

func (g *graphImporter) Import(_ context.Context, w catalogv1.Writer) error {
	for _, v := range g.bundles {
		bid := g.pkg + ".v" + v
		if err := w.InsertBundle(bid, g.pkg, v, "", "docker://example.com/"+g.pkg+":v"+v); err != nil {
			return err
		}
	}
	if err := w.CreateGraph([]string{g.pkg}); err != nil {
		return err
	}
	for _, sg := range g.subGraphs {
		if err := buildSubGraph(w, g.pkg, []string{g.pkg}, sg); err != nil {
			return err
		}
	}
	return nil
}

func buildSubGraph(w catalogv1.Writer, pkg string, parentPath []string, sg graphOption) error {
	path := append(slices.Clone(parentPath), sg.name)
	if err := w.CreateGraph(path); err != nil {
		return err
	}
	for _, v := range sg.bundles {
		if err := w.AddBundleToGraph(path, pkg+".v"+v); err != nil {
			return err
		}
	}
	for _, e := range sg.edges {
		if err := w.AddEdge(path, pkg+".v"+e.from, pkg+".v"+e.to); err != nil {
			return err
		}
	}
	for _, r := range sg.ranges {
		if err := w.AddPredecessorRange(path, pkg+".v"+r.bundle, r.versionRange); err != nil {
			return err
		}
	}
	for _, child := range sg.children {
		if err := buildSubGraph(w, pkg, path, child); err != nil {
			return err
		}
	}
	return nil
}

// --- Helpers ---

func newTempStore(t *testing.T) (catalogv1.Store, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := sqlite.OpenStore(dbPath)
	require.NoError(t, err)
	return store, func() { require.NoError(t, store.Close()) }
}

func importGraph(t *testing.T, store catalogv1.Store, imp catalogv1.Importer) {
	t.Helper()
	_, err := store.Set(context.Background(), "cat",
		catalogv1.WithURI("test://"),
		catalogv1.WithContent(imp, "digest"),
	)
	require.NoError(t, err)
}

func collectBundleIDs(t *testing.T, bundles []bundlev1.Bundle) []string {
	t.Helper()
	var ids []string
	for _, b := range bundles {
		ids = append(ids, string(b.ID()))
	}
	return ids
}

// --- Tests ---

func TestResolve_NoOptions(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0", "2.0.0", "3.0.0"},
		subGraph("stable", []string{"1.0.0", "2.0.0"}),
		subGraph("fast", []string{"2.0.0", "3.0.0"}),
	))

	result, err := resolverv1.Resolve(context.Background(), store, "pkg")
	require.NoError(t, err)
	ids := collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"}, ids)
}

func TestResolve_WithGraphs_Depth1(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0", "2.0.0", "3.0.0"},
		subGraph("sg1", []string{"1.0.0", "2.0.0"}),
		subGraph("sg2", []string{"2.0.0", "3.0.0"}),
	))

	result, err := resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}}),
	)
	require.NoError(t, err)
	ids := collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v2.0.0", "pkg.v1.0.0"}, ids)
}

func TestResolve_WithGraphs_Nonexistent(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0"},
		subGraph("stable", []string{"1.0.0"}),
	))

	result, err := resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithGraphs([][]string{{"nonexistent"}}),
	)
	require.NoError(t, err)
	assert.Empty(t, result.Bundles)
}

func TestResolve_WithGraphs_MultiplePaths(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0", "5.0.0", "6.0.0"},
		subGraph("sg1", []string{"1.0.0", "2.0.0"}),
		subGraph("sg2", []string{"3.0.0", "4.0.0"}),
		subGraph("sg3", []string{"5.0.0", "6.0.0"}),
	))

	// sg1 + sg2 = [1,2,3,4] — sg3 bundles excluded
	result, err := resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}, {"sg2"}}),
	)
	require.NoError(t, err)
	ids := collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"}, ids)

	// sg1 only = [1,2] — sg2 and sg3 bundles excluded
	result, err = resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}}),
	)
	require.NoError(t, err)
	ids = collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v2.0.0", "pkg.v1.0.0"}, ids)
}

func TestResolve_WithGraphs_Depth2(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0", "2.0.0"},
		subGraph("stable", []string{"1.0.0", "2.0.0"},
			withChild(subGraph("lts", []string{"1.0.0"})),
		),
	))

	result, err := resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithGraphs([][]string{{"stable", "lts"}}),
	)
	require.NoError(t, err)
	ids := collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v1.0.0"}, ids)
}

func TestResolve_WithMastermindsVersionConstraint(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0", "1.1.0", "2.0.0", "3.0.0"},
		subGraph("stable", []string{"1.0.0", "1.1.0", "2.0.0", "3.0.0"}),
	))

	constraint, err := mmsemver.NewConstraint(">=1.0.0, <2.0.0")
	require.NoError(t, err)

	result, err := resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithMastermindsVersionConstraint(*constraint),
	)
	require.NoError(t, err)
	ids := collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v1.1.0", "pkg.v1.0.0"}, ids)
}

func TestResolve_WithSuccessorsOf(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0", "1.1.0", "2.0.0", "3.0.0", "4.0.0"},
		subGraph("stable", []string{"1.0.0", "1.1.0", "2.0.0", "3.0.0", "4.0.0"},
			withEdge("1.0.0", "1.1.0"),
			withRange("2.0.0", ">=1.0.0 <2.0.0"),
			withRange("3.0.0", ">=1.0.0 <3.0.0"),
			withEdge("3.0.0", "4.0.0"),
		),
	))

	// 4.0.0 is reachable from 3.0.0 but not from 1.0.0, so it should be excluded
	result, err := resolverv1.Resolve(context.Background(), store, "pkg",
		resolverv1.WithSuccessorsOf(testutil.NewBundleIdentity(t, "pkg", "1.0.0", "")),
	)
	require.NoError(t, err)
	ids := collectBundleIDs(t, result.Bundles)
	assert.Equal(t, []string{"pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.1.0"}, ids)
}

func TestResolve_WithGraphsAndSuccessorsOf(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Three sub-graphs with the same bundles but different edges.
	// Each graph has edges from A and edges NOT from A to prove only
	// A's direct successors are returned.
	//   sg1: A -> B, B -> C, C -> D
	//   sg2: A -> D, B -> D, C -> D
	//   sg3: A -> B, A -> C, B -> C, C -> D
	all := []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0"}
	importGraph(t, store, graph("pkg", all,
		subGraph("sg1", all,
			withEdge("1.0.0", "2.0.0"),
			withEdge("2.0.0", "3.0.0"),
			withEdge("3.0.0", "4.0.0"),
		),
		subGraph("sg2", all,
			withEdge("1.0.0", "4.0.0"),
			withEdge("2.0.0", "4.0.0"),
			withEdge("3.0.0", "4.0.0"),
		),
		subGraph("sg3", all,
			withEdge("1.0.0", "2.0.0"),
			withEdge("1.0.0", "3.0.0"),
			withEdge("2.0.0", "3.0.0"),
			withEdge("3.0.0", "4.0.0"),
		),
	))

	from := testutil.NewBundleIdentity(t, "pkg", "1.0.0", "")

	// All successors of A (no graph filter) = [B, C, D]
	result, err := resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))

	// sg1 -> [B]
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}}),
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))

	// sg2 -> [D]
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg2"}}),
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v4.0.0"}, collectBundleIDs(t, result.Bundles))

	// sg3 -> [B, C]
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg3"}}),
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v3.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))

	// sg2 or sg3 -> [B, C, D]
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg2"}, {"sg3"}}),
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))

	// sg1 or sg2 -> [B, D]
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}, {"sg2"}}),
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_CombinedOptions(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Bundles: A(1.0.0), B(2.0.0), C(3.0.0), D(4.0.0), E(5.0.0), F(6.0.0)
	// sg1: bundles [A-E], edges A->B, A->C, A->D, D->E
	// sg2: bundles [A,D,E,F], edges A->D, A->E
	//
	// No filter:         all bundles [A,B,C,D,E,F]
	// WithGraphs(sg1):   [A,B,C,D,E]  (F excluded: only in sg2)
	// + WithSuccessors:  [B,C,D]      (E excluded: not a direct successor of A in sg1)
	// + WithConstraint:  [B,C]        (D excluded: >=2.0.0 <4.0.0 excludes 4.0.0)
	sg1Bundles := []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0", "5.0.0"}
	sg2Bundles := []string{"1.0.0", "4.0.0", "5.0.0", "6.0.0"}
	importGraph(t, store, graph("pkg", []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0", "5.0.0", "6.0.0"},
		subGraph("sg1", sg1Bundles,
			withEdge("1.0.0", "2.0.0"),
			withEdge("1.0.0", "3.0.0"),
			withEdge("1.0.0", "4.0.0"),
			withEdge("4.0.0", "5.0.0"),
		),
		subGraph("sg2", sg2Bundles,
			withEdge("1.0.0", "4.0.0"),
			withEdge("1.0.0", "5.0.0"),
		),
	))

	from := testutil.NewBundleIdentity(t, "pkg", "1.0.0", "")

	// No filter: all bundles
	result, err := resolverv1.Resolve(ctx, store, "pkg")
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v6.0.0", "pkg.v5.0.0", "pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"},
		collectBundleIDs(t, result.Bundles))

	// WithGraphs(sg1): F filtered out (only in sg2)
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}}),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v5.0.0", "pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0", "pkg.v1.0.0"},
		collectBundleIDs(t, result.Bundles))

	// + WithSuccessorsOf: E filtered out (D->E exists but A->E doesn't in sg1)
	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}}),
		resolverv1.WithSuccessorsOf(from),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v4.0.0", "pkg.v3.0.0", "pkg.v2.0.0"},
		collectBundleIDs(t, result.Bundles))

	// + WithConstraint: D filtered out (4.0.0 doesn't match >=2.0.0 <4.0.0)
	constraint, err := mmsemver.NewConstraint(">=2.0.0, <4.0.0")
	require.NoError(t, err)

	result, err = resolverv1.Resolve(ctx, store, "pkg",
		resolverv1.WithGraphs([][]string{{"sg1"}}),
		resolverv1.WithSuccessorsOf(from),
		resolverv1.WithMastermindsVersionConstraint(*constraint),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.v3.0.0", "pkg.v2.0.0"}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_Priority(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "low-priority",
		catalogv1.WithURI("test://low"),
		catalogv1.WithPriority(1),
		catalogv1.WithContent(graph("pkg", []string{"2.0.0"},
			subGraph("stable", []string{"2.0.0"}),
		), "d1"),
	)
	require.NoError(t, err)

	_, err = store.Set(ctx, "high-priority",
		catalogv1.WithURI("test://high"),
		catalogv1.WithPriority(10),
		catalogv1.WithContent(graph("pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d2"),
	)
	require.NoError(t, err)

	result, err := resolverv1.Resolve(ctx, store, "pkg")
	require.NoError(t, err)
	assert.Equal(t, "high-priority", result.Catalog.Name())
	assert.Equal(t, []string{"pkg.v1.0.0"}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_AmbiguityError(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "cat-a",
		catalogv1.WithURI("test://a"),
		catalogv1.WithPriority(5),
		catalogv1.WithContent(graph("pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d1"),
	)
	require.NoError(t, err)

	_, err = store.Set(ctx, "cat-b",
		catalogv1.WithURI("test://b"),
		catalogv1.WithPriority(5),
		catalogv1.WithContent(graph("pkg", []string{"2.0.0"},
			subGraph("stable", []string{"2.0.0"}),
		), "d2"),
	)
	require.NoError(t, err)

	_, err = resolverv1.Resolve(ctx, store, "pkg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
	assert.Contains(t, err.Error(), "cat-a")
	assert.Contains(t, err.Error(), "cat-b")
}

func TestResolve_AmbiguityError_ThreeCatalogs(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	for _, name := range []string{"cat-c", "cat-a", "cat-b"} {
		_, err := store.Set(ctx, name,
			catalogv1.WithURI("test://"+name),
			catalogv1.WithPriority(5),
			catalogv1.WithContent(graph("pkg", []string{"1.0.0"},
				subGraph("stable", []string{"1.0.0"}),
			), "d"),
		)
		require.NoError(t, err)
	}

	_, err := resolverv1.Resolve(ctx, store, "pkg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
	assert.Contains(t, err.Error(), "[cat-a cat-b cat-c]")
}

func TestResolve_AmbiguitySkipsHigherPriorityGroup(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "high-empty",
		catalogv1.WithURI("test://high"),
		catalogv1.WithPriority(10),
		catalogv1.WithContent(graph("other-pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d1"),
	)
	require.NoError(t, err)

	_, err = store.Set(ctx, "low-a",
		catalogv1.WithURI("test://low-a"),
		catalogv1.WithPriority(5),
		catalogv1.WithContent(graph("pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d2"),
	)
	require.NoError(t, err)

	_, err = store.Set(ctx, "low-b",
		catalogv1.WithURI("test://low-b"),
		catalogv1.WithPriority(5),
		catalogv1.WithContent(graph("pkg", []string{"2.0.0"},
			subGraph("stable", []string{"2.0.0"}),
		), "d3"),
	)
	require.NoError(t, err)

	_, err = resolverv1.Resolve(ctx, store, "pkg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
	assert.Contains(t, err.Error(), "priority 5")
}

func TestResolve_HighPriorityGroupNoMatch_FallsToLower(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "high-empty",
		catalogv1.WithURI("test://high"),
		catalogv1.WithPriority(10),
		catalogv1.WithContent(graph("other-pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d1"),
	)
	require.NoError(t, err)

	_, err = store.Set(ctx, "low",
		catalogv1.WithURI("test://low"),
		catalogv1.WithPriority(5),
		catalogv1.WithContent(graph("pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d2"),
	)
	require.NoError(t, err)

	result, err := resolverv1.Resolve(ctx, store, "pkg")
	require.NoError(t, err)
	assert.Equal(t, "low", result.Catalog.Name())
	assert.Equal(t, []string{"pkg.v1.0.0"}, collectBundleIDs(t, result.Bundles))
}

func TestResolve_NonexistentPackage(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()

	importGraph(t, store, graph("pkg", []string{"1.0.0"},
		subGraph("stable", []string{"1.0.0"}),
	))

	result, err := resolverv1.Resolve(context.Background(), store, "nonexistent-pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolve_Select(t *testing.T) {
	store, cleanup := newTempStore(t)
	defer cleanup()
	ctx := context.Background()

	_, err := store.Set(ctx, "prod-catalog",
		catalogv1.WithURI("test://prod"),
		catalogv1.WithPriority(5),
		catalogv1.WithLabels(map[string]string{"env": "prod"}),
		catalogv1.WithContent(graph("pkg", []string{"1.0.0"},
			subGraph("stable", []string{"1.0.0"}),
		), "d1"),
	)
	require.NoError(t, err)

	_, err = store.Set(ctx, "dev-catalog",
		catalogv1.WithURI("test://dev"),
		catalogv1.WithPriority(5),
		catalogv1.WithLabels(map[string]string{"env": "dev"}),
		catalogv1.WithContent(graph("pkg", []string{"2.0.0"},
			subGraph("stable", []string{"2.0.0"}),
		), "d2"),
	)
	require.NoError(t, err)

	// Without Select, same priority would be ambiguous
	_, err = resolverv1.Resolve(ctx, store, "pkg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")

	// Select narrows to prod-catalog, resolving the ambiguity
	selector, err := labels.Parse("env=prod")
	require.NoError(t, err)

	result, err := resolverv1.Resolve(ctx, store.Select(selector), "pkg")
	require.NoError(t, err)
	assert.Equal(t, "prod-catalog", result.Catalog.Name())
	assert.Equal(t, []string{"pkg.v1.0.0"}, collectBundleIDs(t, result.Bundles))
}
