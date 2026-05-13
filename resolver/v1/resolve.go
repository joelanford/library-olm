package resolverv1

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"slices"
	"strings"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// ResolveOption configures a Resolve operation.
type ResolveOption interface {
	applyResolveOption(*resolveConfig)
}

type resolveConfig struct {
	graphs     [][]string
	constraint *mmsemver.Constraints
	from       bundlev1.BundleIdentity
}

type withGraphs struct{ paths [][]string }

func (o withGraphs) applyResolveOption(c *resolveConfig) { c.graphs = o.paths }

// WithGraphs narrows resolution to the sub-graphs at the given paths.
// Each path is a sequence of graph names walked from the package root.
func WithGraphs(paths [][]string) ResolveOption {
	return withGraphs{paths: paths}
}

type withMastermindsVersionConstraint struct{ constraint mmsemver.Constraints }

func (o withMastermindsVersionConstraint) applyResolveOption(c *resolveConfig) {
	c.constraint = &o.constraint
}

// WithMastermindsVersionConstraint narrows resolution to bundles whose
// version satisfies the given constraint.
func WithMastermindsVersionConstraint(constraint mmsemver.Constraints) ResolveOption {
	return withMastermindsVersionConstraint{constraint: constraint}
}

type withSuccessorsOf struct{ from bundlev1.BundleIdentity }

func (o withSuccessorsOf) applyResolveOption(c *resolveConfig) { c.from = o.from }

// WithSuccessorsOf narrows resolution to bundles that are successors
// of the given bundle.
func WithSuccessorsOf(from bundlev1.BundleIdentity) ResolveOption {
	return withSuccessorsOf{from: from}
}

// Result holds the output of a Resolve call.
type Result struct {
	Catalog catalogv1.Catalog
	Package catalogv1.UpdateGraph
	Bundles []bundlev1.Bundle
}

// Resolve finds bundles matching the given criteria across all catalogs in the
// reader, sorted by version descending. It selects the highest-priority catalog
// containing the package and returns an ambiguity error if multiple catalogs at
// the same priority have it.
func Resolve(ctx context.Context, reader catalogv1.StoreReader, packageName string, opts ...ResolveOption) (*Result, error) {
	var cfg resolveConfig
	for _, opt := range opts {
		opt.applyResolveOption(&cfg)
	}

	catalogs, err := reader.List()
	if err != nil {
		return nil, fmt.Errorf("listing catalogs: %w", err)
	}

	cat, pkg, err := selectPackage(ctx, catalogs, packageName)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, nil
	}

	graphs, err := selectGraphs(ctx, pkg, cfg.graphs)
	if err != nil {
		return nil, err
	}
	if len(graphs) == 0 {
		return &Result{Catalog: cat, Package: pkg}, nil
	}

	bundles, err := collectBundles(ctx, graphs, cfg.from)
	if err != nil {
		return nil, err
	}

	bundles = filterByConstraint(bundles, cfg.constraint)

	slices.SortFunc(bundles, func(a, b bundlev1.Bundle) int {
		return b.NameVersionRelease().Compare(a.NameVersionRelease())
	})
	return &Result{Catalog: cat, Package: pkg, Bundles: bundles}, nil
}

func selectPackage(ctx context.Context, catalogs []catalogv1.Catalog, packageName string) (catalogv1.Catalog, catalogv1.UpdateGraph, error) {
	groups := groupByPriority(catalogs)
	for _, group := range groups {
		type match struct {
			catalog catalogv1.Catalog
			pkg     catalogv1.UpdateGraph
		}
		var matches []match
		for _, cat := range group {
			pkg, err := cat.GetPackage(ctx, packageName)
			if err != nil {
				continue
			}
			matches = append(matches, match{catalog: cat, pkg: pkg})
		}
		if len(matches) == 0 {
			continue
		}
		if len(matches) == 1 {
			return matches[0].catalog, matches[0].pkg, nil
		}
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.catalog.Name()
		}
		slices.Sort(names)
		return nil, nil, fmt.Errorf(
			"ambiguous: package %q found in multiple catalogs at priority %d: %v",
			packageName, group[0].Priority(), names,
		)
	}
	return nil, nil, nil
}

func groupByPriority(catalogs []catalogv1.Catalog) [][]catalogv1.Catalog {
	if len(catalogs) == 0 {
		return nil
	}
	sorted := slices.Clone(catalogs)
	slices.SortFunc(sorted, func(a, b catalogv1.Catalog) int {
		return cmp.Compare(b.Priority(), a.Priority())
	})
	var groups [][]catalogv1.Catalog
	var current []catalogv1.Catalog
	currentPriority := sorted[0].Priority()
	for _, cat := range sorted {
		if cat.Priority() != currentPriority {
			groups = append(groups, current)
			current = nil
			currentPriority = cat.Priority()
		}
		current = append(current, cat)
	}
	return append(groups, current)
}

func selectGraphs(ctx context.Context, pkg catalogv1.UpdateGraph, paths [][]string) ([]catalogv1.UpdateGraph, error) {
	if len(paths) == 0 {
		return []catalogv1.UpdateGraph{pkg}, nil
	}
	var graphs []catalogv1.UpdateGraph
	for _, path := range paths {
		g, err := walkPath(ctx, pkg, path)
		if err != nil {
			return nil, err
		}
		if g != nil {
			graphs = append(graphs, g)
		}
	}
	return graphs, nil
}

func walkPath(ctx context.Context, root catalogv1.UpdateGraph, path []string) (catalogv1.UpdateGraph, error) {
	current := root
	for _, name := range path {
		composite, ok := current.(catalogv1.CompositeUpdateGraph)
		if !ok {
			return nil, nil
		}
		g, err := composite.GetGraph(ctx, name)
		if err != nil {
			return nil, nil
		}
		current = g
	}
	return current, nil
}

func collectBundles(ctx context.Context, graphs []catalogv1.UpdateGraph, from bundlev1.BundleIdentity) ([]bundlev1.Bundle, error) {
	seen := make(map[bundlev1.BundleID]struct{})
	var result []bundlev1.Bundle
	for _, g := range graphs {
		var seq iter.Seq2[bundlev1.Bundle, error]
		if from != nil {
			seq = g.Successors(ctx, from)
		} else {
			seq = g.ListBundles(ctx)
		}
		for b, err := range seq {
			if err != nil {
				return nil, err
			}
			if _, ok := seen[b.ID()]; ok {
				continue
			}
			seen[b.ID()] = struct{}{}
			result = append(result, b)
		}
	}
	return result, nil
}

func filterByConstraint(bundles []bundlev1.Bundle, constraint *mmsemver.Constraints) []bundlev1.Bundle {
	if constraint == nil {
		return bundles
	}
	var filtered []bundlev1.Bundle
	for _, b := range bundles {
		v := b.NameVersionRelease().Version
		mmv := blangToMasterminds(v)
		if constraint.Check(mmv) {
			filtered = append(filtered, b)
		}
	}
	return filtered
}

func blangToMasterminds(v bsemver.Version) *mmsemver.Version {
	preStrs := make([]string, len(v.Pre))
	for i, p := range v.Pre {
		preStrs[i] = p.String()
	}
	return mmsemver.New(v.Major, v.Minor, v.Patch, strings.Join(preStrs, "."), strings.Join(v.Build, "."))
}
