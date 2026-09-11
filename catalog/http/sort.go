package cataloghttp

import (
	"cmp"
	"slices"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

func sortCatalogs(catalogs []catalogv1.Catalog) {
	slices.SortFunc(catalogs, func(a, b catalogv1.Catalog) int {
		return cmp.Compare(a.Name(), b.Name())
	})
}

func sortPackageSummaries(packages []packageSummary) {
	slices.SortFunc(packages, func(a, b packageSummary) int {
		if order := cmp.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		if order := cmp.Compare(b.CatalogPriority, a.CatalogPriority); order != 0 {
			return order
		}
		return cmp.Compare(a.CatalogName, b.CatalogName)
	})
}

func sortChannelSummaries(channels []channelSummary) {
	slices.SortFunc(channels, func(a, b channelSummary) int {
		return slices.Compare(a.Path, b.Path)
	})
}

func sortBundles(bundles []bundlev1.Bundle) {
	slices.SortFunc(bundles, func(a, b bundlev1.Bundle) int {
		aNVR := a.NameVersionRelease()
		bNVR := b.NameVersionRelease()
		if order := bNVR.Version.Compare(aNVR.Version); order != 0 {
			return order
		}
		if order := bNVR.Release.Compare(aNVR.Release); order != 0 {
			return order
		}
		return cmp.Compare(a.ID(), b.ID())
	})
}
