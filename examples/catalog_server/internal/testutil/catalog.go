package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/labels"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
)

type StoreReader struct {
	Catalogs      []catalogv1.Catalog
	ListErr       error
	ListErrAfter  int
	GetErr        error
	ListCalls     int
	SelectCalls   []string
	LastSelection *StoreReader
	base          *StoreReader
	selector      labels.Selector
}

func (r *StoreReader) Get(name string) (catalogv1.Catalog, error) {
	if r.GetErr != nil {
		return nil, r.GetErr
	}
	for _, catalog := range r.catalogs() {
		if catalog.Name() == name {
			return catalog, nil
		}
	}
	return nil, fmt.Errorf("catalog %q: %w", name, catalogv1.ErrNotFound)
}

func (r *StoreReader) List() ([]catalogv1.Catalog, error) {
	r.ListCalls++
	if r.ListErrAfter > 0 && r.ListCalls <= r.ListErrAfter {
		return slices.Clone(r.catalogs()), nil
	}
	return slices.Clone(r.catalogs()), r.ListErr
}

func (r *StoreReader) Select(selector labels.Selector) catalogv1.StoreReader {
	r.SelectCalls = append(r.SelectCalls, selector.String())
	base := r
	if r.base != nil {
		base = r.base
		requirements, _ := selector.Requirements()
		selector = r.selector.Add(requirements...)
	}
	selected := &StoreReader{ListErr: r.ListErr, ListErrAfter: r.ListErrAfter, GetErr: r.GetErr, base: base, selector: selector}
	r.LastSelection = selected
	return selected
}

func (r *StoreReader) catalogs() []catalogv1.Catalog {
	if r.base == nil {
		return r.Catalogs
	}
	selected := make([]catalogv1.Catalog, 0, len(r.base.Catalogs))
	for _, catalog := range r.base.Catalogs {
		if r.selector.Matches(labels.Set(catalog.Labels())) {
			selected = append(selected, catalog)
		}
	}
	return selected
}

type Catalog struct {
	CatalogName       string
	CatalogURI        string
	CatalogDigest     string
	CatalogPriority   int
	CatalogLabels     map[string]string
	Packages          map[string]catalogv1.UpdateGraph
	ListPackagesErr   error
	GetPackageErr     error
	ListPackagesCalls int
	GetPackageCalls   []string
}

func (c *Catalog) Name() string              { return c.CatalogName }
func (c *Catalog) URI() string               { return c.CatalogURI }
func (c *Catalog) Digest() string            { return c.CatalogDigest }
func (c *Catalog) Priority() int             { return c.CatalogPriority }
func (c *Catalog) Labels() map[string]string { return maps.Clone(c.CatalogLabels) }

func (c *Catalog) ListPackages(context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	c.ListPackagesCalls++
	return func(yield func(catalogv1.UpdateGraph, error) bool) {
		names := make([]string, 0, len(c.Packages))
		for name := range c.Packages {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			if !yield(c.Packages[name], nil) {
				return
			}
		}
		if c.ListPackagesErr != nil {
			yield(nil, c.ListPackagesErr)
		}
	}
}

func (c *Catalog) GetPackage(_ context.Context, name string) (catalogv1.UpdateGraph, error) {
	c.GetPackageCalls = append(c.GetPackageCalls, name)
	if c.GetPackageErr != nil {
		return nil, c.GetPackageErr
	}
	if pkg, ok := c.Packages[name]; ok {
		return pkg, nil
	}
	return nil, fmt.Errorf("package %q: %w", name, catalogv1.ErrNotFound)
}

type LeafGraph struct {
	GraphName        string
	Bundles          []bundlev1.Bundle
	SuccessorBundles []bundlev1.Bundle
	ListBundlesErr   error
	SuccessorsErr    error
	ListBundlesCalls int
	SuccessorsCalls  []bundlev1.BundleIdentity
	Properties       map[string]json.RawMessage
	PropertyErr      error
}

func (g *LeafGraph) Name() string { return g.GraphName }

func (g *LeafGraph) ListBundles(context.Context) iter.Seq2[bundlev1.Bundle, error] {
	g.ListBundlesCalls++
	return valuesOrError(g.Bundles, g.ListBundlesErr)
}

func (g *LeafGraph) Successors(_ context.Context, from bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	g.SuccessorsCalls = append(g.SuccessorsCalls, from)
	return valuesOrError(g.SuccessorBundles, g.SuccessorsErr)
}

func (g *LeafGraph) Property(_ context.Context, key string) (json.RawMessage, error) {
	return g.Properties[key], g.PropertyErr
}

type Package struct {
	*LeafGraph
	PackageMetadata model.PackageMetadata
	PackageIcon     model.Icon
	MetadataErr     error
	IconErr         error
	MetadataCalls   int
	IconCalls       int
}

func (p *Package) Property(ctx context.Context, key string) (json.RawMessage, error) {
	switch key {
	case model.PackageMetadataProperty:
		p.MetadataCalls++
		if p.MetadataErr != nil {
			return nil, p.MetadataErr
		}
		return json.Marshal(p.PackageMetadata)
	case model.PackageIconProperty:
		p.IconCalls++
		if p.IconErr != nil {
			return nil, p.IconErr
		}
		if p.PackageIcon.Content == nil {
			return nil, nil
		}
		return json.Marshal(p.PackageIcon)
	default:
		return p.LeafGraph.Property(ctx, key)
	}
}

type CompositeGraph struct {
	*LeafGraph
	Graphs          map[string]catalogv1.UpdateGraph
	ListGraphsErr   error
	GetGraphErr     error
	ListGraphsCalls int
	GetGraphCalls   []string
}

func (g *CompositeGraph) ListGraphs(context.Context) iter.Seq2[catalogv1.UpdateGraph, error] {
	g.ListGraphsCalls++
	return func(yield func(catalogv1.UpdateGraph, error) bool) {
		if g.ListGraphsErr != nil {
			yield(nil, g.ListGraphsErr)
			return
		}
		names := make([]string, 0, len(g.Graphs))
		for name := range g.Graphs {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			if !yield(g.Graphs[name], nil) {
				return
			}
		}
	}
}

func (g *CompositeGraph) GetGraph(_ context.Context, name string) (catalogv1.UpdateGraph, error) {
	g.GetGraphCalls = append(g.GetGraphCalls, name)
	if g.GetGraphErr != nil {
		return nil, g.GetGraphErr
	}
	if graph, ok := g.Graphs[name]; ok {
		return graph, nil
	}
	return nil, fmt.Errorf("graph %q: %w", name, catalogv1.ErrNotFound)
}

type CompositePackage struct {
	*CompositeGraph
	PackageMetadata model.PackageMetadata
	PackageIcon     model.Icon
	MetadataErr     error
	IconErr         error
	MetadataCalls   int
	IconCalls       int
}

func (p *CompositePackage) Property(ctx context.Context, key string) (json.RawMessage, error) {
	switch key {
	case model.PackageMetadataProperty:
		p.MetadataCalls++
		if p.MetadataErr != nil {
			return nil, p.MetadataErr
		}
		return json.Marshal(p.PackageMetadata)
	case model.PackageIconProperty:
		p.IconCalls++
		if p.IconErr != nil {
			return nil, p.IconErr
		}
		if p.PackageIcon.Content == nil {
			return nil, nil
		}
		return json.Marshal(p.PackageIcon)
	default:
		return p.CompositeGraph.Property(ctx, key)
	}
}

func valuesOrError[T any](values []T, err error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for _, value := range values {
			if !yield(value, nil) {
				return
			}
		}
		if err != nil {
			var zero T
			yield(zero, err)
		}
	}
}

var _ catalogv1.StoreReader = (*StoreReader)(nil)
var _ catalogv1.Catalog = (*Catalog)(nil)
var _ catalogv1.UpdateGraph = (*LeafGraph)(nil)
var _ catalogv1.CompositeUpdateGraph = (*CompositeGraph)(nil)
var _ catalogv1.UpdateGraph = (*Package)(nil)
var _ catalogv1.CompositeUpdateGraph = (*CompositePackage)(nil)
