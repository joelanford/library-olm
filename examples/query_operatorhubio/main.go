package main

import (
	"context"
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
		return fmt.Errorf("load catalog: %w", err)
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

	// GetPackage
	target := pkgNames[0]
	log.Printf("Getting package %q...", target)
	start = time.Now()
	pkg, err := cat.GetPackage(ctx, target)
	if err != nil {
		return fmt.Errorf("get package %q: %w", target, err)
	}
	log.Printf("Got package %q in %s", pkg.Name(), time.Since(start))

	// CompositeUpdateGraph: ListGraphs
	composite, ok := pkg.(catalogv1.CompositeUpdateGraph)
	if !ok {
		log.Printf("Package %q is not a CompositeUpdateGraph, skipping channel queries", target)
		return nil
	}

	log.Printf("Listing graphs (channels) for package %q...", target)
	start = time.Now()
	var graphNames []string
	for g, err := range composite.ListGraphs(ctx) {
		if err != nil {
			return fmt.Errorf("list graphs: %w", err)
		}
		graphNames = append(graphNames, g.Name())
	}
	log.Printf("Found %d graphs in %s: %v", len(graphNames), time.Since(start), graphNames)

	// CompositeUpdateGraph: ListBundles (package-level, union across channels)
	log.Printf("Listing all bundles in package %q (union across channels)...", target)
	start = time.Now()
	var pkgBundleCount int
	var sampleBundle bundlev1.Bundle
	for b, err := range composite.ListBundles(ctx) {
		if err != nil {
			return fmt.Errorf("list bundles (package): %w", err)
		}
		pkgBundleCount++
		if sampleBundle == nil {
			sampleBundle = b
		}
	}
	log.Printf("Found %d bundles (package-level) in %s", pkgBundleCount, time.Since(start))

	// CompositeUpdateGraph: GetGraph
	if len(graphNames) > 0 {
		chName := graphNames[0]
		log.Printf("Getting graph %q...", chName)
		start = time.Now()
		ch, err := composite.GetGraph(ctx, chName)
		if err != nil {
			return fmt.Errorf("get graph %q: %w", chName, err)
		}
		log.Printf("Got graph %q in %s", ch.Name(), time.Since(start))

		// UpdateGraph: ListBundles (channel-level)
		log.Printf("Listing bundles in channel %q...", chName)
		start = time.Now()
		var chBundleCount int
		var chFirstBundle, chLastBundle bundlev1.Bundle
		for b, err := range ch.ListBundles(ctx) {
			if err != nil {
				return fmt.Errorf("list bundles (channel): %w", err)
			}
			chBundleCount++
			if chFirstBundle == nil {
				chFirstBundle = b
			}
			chLastBundle = b
		}
		log.Printf("Found %d bundles in channel %q in %s", chBundleCount, chName, time.Since(start))

		if chFirstBundle != nil {
			vr := chFirstBundle.VersionRelease()
			log.Printf("  First bundle: %s (version=%s, release=%s)", chFirstBundle.Name(), vr.Version, vr.Release)
		}
		if chLastBundle != nil {
			vr := chLastBundle.VersionRelease()
			log.Printf("  Last bundle:  %s (version=%s, release=%s)", chLastBundle.Name(), vr.Version, vr.Release)
		}

		// UpdateGraph: Successors (channel-level)
		if chFirstBundle != nil {
			log.Printf("Querying successors of %q in channel %q...", chFirstBundle.Name(), chName)
			start = time.Now()
			var successorCount int
			for s, err := range ch.Successors(ctx, chFirstBundle) {
				if err != nil {
					return fmt.Errorf("successors: %w", err)
				}
				successorCount++
				if successorCount <= 3 {
					vr := s.VersionRelease()
					log.Printf("  Successor: %s (version=%s)", s.Name(), vr.Version)
				}
			}
			if successorCount > 3 {
				log.Printf("  ... and %d more", successorCount-3)
			}
			log.Printf("Found %d successors in %s", successorCount, time.Since(start))
		}

		// CompositeUpdateGraph: Successors (package-level)
		if chFirstBundle != nil {
			log.Printf("Querying successors of %q across all channels...", chFirstBundle.Name())
			start = time.Now()
			var pkgSuccessorCount int
			for _, err := range composite.Successors(ctx, chFirstBundle) {
				if err != nil {
					return fmt.Errorf("successors (package): %w", err)
				}
				pkgSuccessorCount++
			}
			log.Printf("Found %d successors (package-level) in %s", pkgSuccessorCount, time.Since(start))
		}
	}

	// GetPackage for unknown package (error case)
	log.Println("Querying non-existent package...")
	start = time.Now()
	_, err = cat.GetPackage(ctx, "this-package-does-not-exist-12345")
	if err != nil {
		log.Printf("Expected error for unknown package: %v (took %s)", err, time.Since(start))
	}

	log.Println("Done!")
	return nil
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
