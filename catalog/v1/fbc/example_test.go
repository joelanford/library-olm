package fbc_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing/fstest"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/fbc"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
)

func ExampleNewImporter() {
	// Build an FBC catalog in memory using NDJSON.
	fsys := fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			`{"schema":"olm.package","name":"my-operator"}` + "\n" +
				`{"schema":"olm.channel","package":"my-operator","name":"stable","entries":[{"name":"my-operator.v1.0.0"},{"name":"my-operator.v1.1.0","replaces":"my-operator.v1.0.0"}]}` + "\n" +
				`{"schema":"olm.channel","package":"my-operator","name":"fast","entries":[{"name":"my-operator.v1.0.0"},{"name":"my-operator.v1.2.0","replaces":"my-operator.v1.0.0"}]}` + "\n" +
				`{"schema":"olm.bundle","package":"my-operator","name":"my-operator.v1.0.0","image":"quay.io/my-operator/bundle:v1.0.0","properties":[{"type":"olm.package","value":{"packageName":"my-operator","version":"1.0.0"}}]}` + "\n" +
				`{"schema":"olm.bundle","package":"my-operator","name":"my-operator.v1.1.0","image":"quay.io/my-operator/bundle:v1.1.0","properties":[{"type":"olm.package","value":{"packageName":"my-operator","version":"1.1.0"}}]}` + "\n" +
				`{"schema":"olm.bundle","package":"my-operator","name":"my-operator.v1.2.0","image":"quay.io/my-operator/bundle:v1.2.0","properties":[{"type":"olm.package","value":{"packageName":"my-operator","version":"1.2.0"}}]}` + "\n",
		)},
	}

	ctx := context.Background()

	// Open a persistent store (use a temp file for the example).
	dbPath := filepath.Join(os.TempDir(), "example-catalog.db")
	defer func() { _ = os.Remove(dbPath) }()

	store, err := sqlite.OpenStore(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	// Import FBC content into the store.
	cat, err := store.Set(ctx, "my-catalog",
		catalogv1.WithURI("test://example"),
		catalogv1.WithContent(fbc.NewImporter(fsys), "example-digest"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// List packages.
	for pkg, err := range cat.ListPackages(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("package:", pkg.Name())
	}

	// Get a package and explore its channels (CompositeUpdateGraph).
	pkg, err := cat.GetPackage(ctx, "my-operator")
	if err != nil {
		log.Fatal(err)
	}
	composite := pkg.(catalogv1.CompositeUpdateGraph)

	var channelNames []string
	for ch, err := range composite.ListGraphs(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		channelNames = append(channelNames, ch.Name())
	}
	slices.Sort(channelNames)
	fmt.Println("channels:", channelNames)

	// List bundles across all channels.
	var bundleIDs []string
	for b, err := range composite.ListBundles(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		bundleIDs = append(bundleIDs, string(b.ID()))
	}
	slices.Sort(bundleIDs)
	fmt.Println("all bundles:", bundleIDs)

	// Query the stable channel's upgrade graph.
	stable, err := composite.GetGraph(ctx, "stable")
	if err != nil {
		log.Fatal(err)
	}

	var stableBundles []string
	for b, err := range stable.ListBundles(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		nvr := b.NameVersionRelease()
		stableBundles = append(stableBundles, fmt.Sprintf("%s (%s)", b.ID(), nvr.Version))
	}
	slices.Sort(stableBundles)
	fmt.Println("stable bundles:", stableBundles)

	// Find successors (upgrade targets) from v1.0.0 in the stable channel.
	// Look up the installed bundle in the graph to get a BundleIdentity.
	installed, err := findBundle(stable.ListBundles(ctx), "my-operator.v1.0.0")
	if err != nil {
		log.Fatal(err)
	}
	var successors []string
	for b, err := range stable.Successors(ctx, installed) {
		if err != nil {
			log.Fatal(err)
		}
		successors = append(successors, string(b.ID()))
	}
	fmt.Println("successors of v1.0.0 in stable:", successors)

	// Output:
	// package: my-operator
	// channels: [fast stable]
	// all bundles: [my-operator.v1.0.0 my-operator.v1.1.0 my-operator.v1.2.0]
	// stable bundles: [my-operator.v1.0.0 (1.0.0) my-operator.v1.1.0 (1.1.0)]
	// successors of v1.0.0 in stable: [my-operator.v1.1.0]
}

func findBundle(seq func(func(bundlev1.Bundle, error) bool), id string) (bundlev1.Bundle, error) {
	for b, err := range seq {
		if err != nil {
			return nil, err
		}
		if string(b.ID()) == id {
			return b, nil
		}
	}
	return nil, fmt.Errorf("bundle %q not found", id)
}
