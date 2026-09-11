package cataloghttp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	testutil "github.com/joelanford/library-olm/internal/util/test"
)

func TestDeterministicSortHelpers(t *testing.T) {
	t.Parallel()

	t.Run("catalogs", func(t *testing.T) {
		catalogs := []catalogv1.Catalog{
			&testutil.Catalog{CatalogName: "z"},
			&testutil.Catalog{CatalogName: "a"},
		}
		sortCatalogs(catalogs)
		assert.Equal(t, []string{"a", "z"}, []string{catalogs[0].Name(), catalogs[1].Name()})
	})

	t.Run("packages", func(t *testing.T) {
		packages := []packageSummary{
			{Name: "z", CatalogPriority: 1, CatalogName: "a"},
			{Name: "a", CatalogPriority: 1, CatalogName: "z"},
			{Name: "a", CatalogPriority: 2, CatalogName: "z"},
			{Name: "a", CatalogPriority: 2, CatalogName: "a"},
		}
		sortPackageSummaries(packages)
		assert.Equal(t, []packageSummary{
			{Name: "a", CatalogPriority: 2, CatalogName: "a"},
			{Name: "a", CatalogPriority: 2, CatalogName: "z"},
			{Name: "a", CatalogPriority: 1, CatalogName: "z"},
			{Name: "z", CatalogPriority: 1, CatalogName: "a"},
		}, packages)
	})

	t.Run("channel paths", func(t *testing.T) {
		channels := []channelSummary{
			{Path: []string{"stable", "2"}},
			{Path: []string{"beta"}},
			{Path: []string{"stable", "1"}},
			{Path: []string{"stable"}},
		}
		sortChannelSummaries(channels)
		assert.Equal(t, [][]string{{"beta"}, {"stable"}, {"stable", "1"}, {"stable", "2"}}, []([]string){
			channels[0].Path, channels[1].Path, channels[2].Path, channels[3].Path,
		})
	})

	t.Run("bundles", func(t *testing.T) {
		bundles := []bundlev1.Bundle{
			testutil.NewBundle(t, "p", "1.0.0", "2"),
			testutil.NewBundle(t, "p", "2.0.0", ""),
			testutil.NewBundle(t, "p", "1.0.0", "10"),
			testutil.NewBundle(t, "p", "1.0.0", "2"),
		}
		bundles[0].(*testutil.Bundle).BundleIdentity = testutil.BundleIdentity{
			BundleID: "z", NVR: bundles[0].NameVersionRelease(),
		}
		bundles[3].(*testutil.Bundle).BundleIdentity = testutil.BundleIdentity{
			BundleID: "a", NVR: bundles[3].NameVersionRelease(),
		}
		sortBundles(bundles)
		ids := make([]bundlev1.BundleID, len(bundles))
		for i, bundle := range bundles {
			ids[i] = bundle.ID()
		}
		assert.Equal(t, []bundlev1.BundleID{"p.v2.0.0", "p.v1.0.0-10", "a", "z"}, ids)
	})
}
