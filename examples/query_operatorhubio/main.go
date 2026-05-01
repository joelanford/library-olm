package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/types"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/image"
	imgcatalog "github.com/joelanford/library-olm/image/catalog"
)

const catalogImage = "//quay.io/operatorhubio/catalog:latest"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	// Step 1: Extract the catalog image to a temp directory
	tmpDir, err := os.MkdirTemp("", "operatorhubio-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	log.Printf("Extracting %s to %s", catalogImage, tmpDir)
	start := time.Now()
	if err := extractCatalogImage(ctx, tmpDir); err != nil {
		return fmt.Errorf("extract catalog image: %w", err)
	}
	log.Printf("Extracted catalog image in %s", time.Since(start))

	// Step 2: Open the extracted catalog as an FBC Catalog
	log.Println("Loading catalog from filesystem into SQLite...")
	start = time.Now()
	cat, err := fbc.FromFS(ctx, os.DirFS(tmpDir))
	if err != nil {
		if cat == nil {
			return fmt.Errorf("load catalog: %w", err)
		}
		var pkgErr *fbc.PackageError
		for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
			if errors.As(e, &pkgErr) {
				log.Printf("WARNING: %v", pkgErr)
			}
		}
	}
	defer func() { _ = cat.Close() }()
	log.Printf("Loaded catalog in %s", time.Since(start))

	// Step 3: Exercise the full API surface

	// ListPackages
	log.Println("Listing all packages...")
	start = time.Now()
	var pkgNames []string
	for pkg, err := range cat.ListPackages(ctx) {
		if err != nil {
			return fmt.Errorf("list packages: %w", err)
		}
		pkgNames = append(pkgNames, pkg.Name())
	}
	log.Printf("Found %d packages in %s", len(pkgNames), time.Since(start))

	if len(pkgNames) == 0 {
		log.Println("No packages found, exiting")
		return nil
	}

	// Find the package with the most channels and bundles
	target, err := findInterestingPackage(ctx, cat, pkgNames)
	if err != nil {
		return err
	}

	pkg, err := cat.GetPackage(ctx, target)
	if err != nil {
		return fmt.Errorf("get package %q: %w", target, err)
	}

	composite, ok := pkg.(catalogv1.CompositeUpdateGraph)
	if !ok {
		log.Printf("Package %q is not a CompositeUpdateGraph, skipping channel queries", target)
		return nil
	}

	// ListGraphs
	log.Printf("Listing graphs (channels) for package %q...", target)
	start = time.Now()
	var graphNames []string
	for g, err := range composite.ListGraphs(ctx) {
		if err != nil {
			return fmt.Errorf("list graphs: %w", err)
		}
		graphNames = append(graphNames, g.Name())
	}
	log.Printf("Found %d channels in %s: %v", len(graphNames), time.Since(start), graphNames)

	// ListBundles (package-level)
	log.Printf("Listing all bundles in package %q...", target)
	start = time.Now()
	var pkgBundleCount int
	for _, err := range composite.ListBundles(ctx) {
		if err != nil {
			return fmt.Errorf("list bundles (package): %w", err)
		}
		pkgBundleCount++
	}
	log.Printf("Found %d bundles (package-level) in %s", pkgBundleCount, time.Since(start))

	// Exercise each channel
	for _, chName := range graphNames {
		ch, err := composite.GetGraph(ctx, chName)
		if err != nil {
			return fmt.Errorf("get graph %q: %w", chName, err)
		}

		var chBundleCount int
		var chFirstBundle, chLastBundle bundlev1.Bundle
		for b, err := range ch.ListBundles(ctx) {
			if err != nil {
				return fmt.Errorf("list bundles (channel %q): %w", chName, err)
			}
			chBundleCount++
			if chFirstBundle == nil {
				chFirstBundle = b
			}
			chLastBundle = b
		}

		first, last := "", ""
		if chFirstBundle != nil {
			first = fmt.Sprintf("%s (%s)", chFirstBundle.Name(), chFirstBundle.VersionRelease().Version)
		}
		if chLastBundle != nil {
			last = fmt.Sprintf("%s (%s)", chLastBundle.Name(), chLastBundle.VersionRelease().Version)
		}
		log.Printf("  Channel %q: %d bundles, first=%s, last=%s", chName, chBundleCount, first, last)

		// Successors from oldest bundle in this channel
		if chFirstBundle != nil {
			var successorCount int
			for _, err := range ch.Successors(ctx, chFirstBundle) {
				if err != nil {
					return fmt.Errorf("successors in channel %q: %w", chName, err)
				}
				successorCount++
			}
			log.Printf("    %d successors from %s", successorCount, chFirstBundle.Name())
		}
	}

	// Package-level successors from the first bundle of the first channel
	if len(graphNames) > 0 {
		ch, err := composite.GetGraph(ctx, graphNames[0])
		if err != nil {
			return fmt.Errorf("get graph %q: %w", graphNames[0], err)
		}
		var firstBundle bundlev1.Bundle
		for b, err := range ch.ListBundles(ctx) {
			if err != nil {
				return fmt.Errorf("list bundles: %w", err)
			}
			firstBundle = b
			break
		}
		if firstBundle != nil {
			start = time.Now()
			var count int
			for _, err := range composite.Successors(ctx, firstBundle) {
				if err != nil {
					return fmt.Errorf("successors (package): %w", err)
				}
				count++
			}
			log.Printf("Package-level: %d successors from %s in %s", count, firstBundle.Name(), time.Since(start))
		}
	}

	// GetPackage for unknown package (error case)
	_, err = cat.GetPackage(ctx, "this-package-does-not-exist-12345")
	if err != nil {
		log.Printf("Expected error for unknown package: %v", err)
	}

	log.Println("Done!")
	return nil
}

func findInterestingPackage(ctx context.Context, cat *fbc.Catalog, pkgNames []string) (string, error) {
	log.Println("Scanning packages to find one with multiple channels and many bundles...")
	start := time.Now()

	bestName := pkgNames[0]
	bestChannels, bestBundles := 0, 0

	for _, name := range pkgNames {
		pkg, err := cat.GetPackage(ctx, name)
		if err != nil {
			continue
		}
		composite, ok := pkg.(catalogv1.CompositeUpdateGraph)
		if !ok {
			continue
		}

		var channels, bundles int
		for _, err := range composite.ListGraphs(ctx) {
			if err != nil {
				break
			}
			channels++
		}
		for _, err := range composite.ListBundles(ctx) {
			if err != nil {
				break
			}
			bundles++
		}

		if channels > bestChannels || (channels == bestChannels && bundles > bestBundles) {
			bestName, bestChannels, bestBundles = name, channels, bundles
		}
	}

	log.Printf("Selected package %q (%d channels, %d bundles) in %s", bestName, bestChannels, bestBundles, time.Since(start))
	return bestName, nil
}

func extractCatalogImage(ctx context.Context, dest string) error {
	imgRef, err := docker.ParseReference(catalogImage)
	if err != nil {
		return fmt.Errorf("parse reference: %w", err)
	}

	client, err := image.NewContainersImageClient(ctx, imgRef, &types.SystemContext{})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer func() { _ = client.Close() }()

	desc, err := client.Resolve(ctx)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	manifestBytes, _, err := client.FetchManifest(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetch manifest: %w", err)
	}

	handler := &imgcatalog.FBCHandler{}
	matches, err := handler.Matches(ctx, client, desc, manifestBytes)
	if err != nil {
		return fmt.Errorf("matches: %w", err)
	}
	if !matches {
		return fmt.Errorf("image does not match FBC handler")
	}

	return handler.Unpack(ctx, client, desc, manifestBytes, dest)
}
