package fbc_test

import (
	"context"
	"fmt"
	"log"
	"slices"
	"testing/fstest"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

func ExampleFromFS() {
	// Build an FBC catalog in memory using NDJSON.
	fsys := fstest.MapFS{
		"catalog.json": &fstest.MapFile{Data: []byte(
			`{"schema":"olm.package","name":"my-operator"}` + "\n" +
				`{"schema":"olm.channel","package":"my-operator","name":"stable","entries":[{"name":"my-operator.v1.0.0"},{"name":"my-operator.v1.1.0","replaces":"my-operator.v1.0.0"}]}` + "\n" +
				`{"schema":"olm.channel","package":"my-operator","name":"fast","entries":[{"name":"my-operator.v1.0.0"},{"name":"my-operator.v1.2.0","replaces":"my-operator.v1.0.0"}]}` + "\n" +
				`{"schema":"olm.bundle","package":"my-operator","name":"my-operator.v1.0.0","properties":[{"type":"olm.package","value":{"packageName":"my-operator","version":"1.0.0"}}]}` + "\n" +
				`{"schema":"olm.bundle","package":"my-operator","name":"my-operator.v1.1.0","properties":[{"type":"olm.package","value":{"packageName":"my-operator","version":"1.1.0"}}]}` + "\n" +
				`{"schema":"olm.bundle","package":"my-operator","name":"my-operator.v1.2.0","properties":[{"type":"olm.package","value":{"packageName":"my-operator","version":"1.2.0"}}]}` + "\n",
		)},
	}

	ctx := context.Background()
	cat, err := fbc.FromFS(ctx, fsys)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := cat.Close(); err != nil {
			log.Fatal(err)
		}
	}()

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
	var bundleNames []string
	for b, err := range composite.ListBundles(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		bundleNames = append(bundleNames, b.Name())
	}
	slices.Sort(bundleNames)
	fmt.Println("all bundles:", bundleNames)

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
		stableBundles = append(stableBundles, fmt.Sprintf("%s (%s)", b.Name(), b.VersionRelease().Version))
	}
	slices.Sort(stableBundles)
	fmt.Println("stable bundles:", stableBundles)

	// Find successors (upgrade targets) from v1.0.0 in the stable channel.
	var v100 = findBundle(ctx, stable, "my-operator.v1.0.0")
	var successors []string
	for b, err := range stable.Successors(ctx, v100) {
		if err != nil {
			log.Fatal(err)
		}
		successors = append(successors, b.Name())
	}
	fmt.Println("successors of v1.0.0 in stable:", successors)

	// Output:
	// package: my-operator
	// channels: [fast stable]
	// all bundles: [my-operator.v1.0.0 my-operator.v1.1.0 my-operator.v1.2.0]
	// stable bundles: [my-operator.v1.0.0 (1.0.0) my-operator.v1.1.0 (1.1.0)]
	// successors of v1.0.0 in stable: [my-operator.v1.1.0]
}

func findBundle(ctx context.Context, g catalogv1.UpdateGraph, name string) bundlev1.Bundle {
	for b, err := range g.ListBundles(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		if b.Name() == name {
			return b
		}
	}
	log.Fatalf("bundle %q not found", name)
	return nil
}
