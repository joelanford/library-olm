package catalogv1

import (
	"context"
	"iter"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
)

// UpdateGraph is a named collection of bundles with upgrade relationships.
// It is the fundamental query primitive: callers use it to list available
// bundles and to ask "what can I upgrade to from here?"
type UpdateGraph interface {
	Name() string
	ListBundles(ctx context.Context) iter.Seq2[bundlev1.Bundle, error]
	Successors(ctx context.Context, from bundlev1.BundleID) iter.Seq2[bundlev1.Bundle, error]
}

// CompositeUpdateGraph is an UpdateGraph composed of named child UpdateGraphs.
// Catalog formats with channel-like concepts (e.g., FBC) implement this so
// callers can discover and query individual channels. Formats without channels
// return a plain UpdateGraph instead.
//
// ListBundles and Successors on a CompositeUpdateGraph operate across all
// child graphs (the union).
type CompositeUpdateGraph interface {
	UpdateGraph
	ListGraphs(ctx context.Context) iter.Seq2[UpdateGraph, error]
	GetGraph(ctx context.Context, name string) (UpdateGraph, error)
}

// Catalog is the top-level entry point for querying a catalog.
// Each package is represented as an UpdateGraph. Implementations backed
// by formats with channel concepts (e.g., FBC) return CompositeUpdateGraphs
// from these methods.
type Catalog interface {
	ListPackages(ctx context.Context) iter.Seq2[UpdateGraph, error]
	GetPackage(ctx context.Context, name string) (UpdateGraph, error)
}
