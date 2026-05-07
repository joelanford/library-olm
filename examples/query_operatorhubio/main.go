package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/types"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/image"
	imgbundle "github.com/joelanford/library-olm/image/bundle"
	imgcatalog "github.com/joelanford/library-olm/image/catalog"
)

const catalogImage = "docker://quay.io/operatorhubio/catalog:latest"

const dockerScheme = "docker://"

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

	catalogDir := filepath.Join(tmpDir, "catalog")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		return fmt.Errorf("create catalog dir: %w", err)
	}
	log.Printf("Extracting %s to %s", catalogImage, catalogDir)
	start := time.Now()
	catalogDigest, err := unpackImage(ctx, catalogImage, &imgcatalog.FBCHandler{}, catalogDir)
	if err != nil {
		return fmt.Errorf("extract catalog image: %w", err)
	}
	log.Printf("Extracted catalog image in %s (digest: %s)", time.Since(start), catalogDigest)

	// Step 2: Open the extracted catalog as an FBC Catalog
	log.Println("Loading catalog from filesystem into SQLite...")
	start = time.Now()
	store, err := catalogv1.OpenStore(filepath.Join(tmpDir, "catalog.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	cat, err := store.Set(ctx, "operatorhubio",
		catalogv1.WithURI(catalogImage),
		catalogv1.WithContent(fbc.NewImporter(os.DirFS(catalogDir)), catalogDigest),
	)
	if err != nil {
		var pkgErr *fbc.PackageError
		if !errors.As(err, &pkgErr) {
			return fmt.Errorf("import catalog: %w", err)
		}
		for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
			if errors.As(e, &pkgErr) {
				log.Printf("WARNING: %v", pkgErr)
			}
		}
	}
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
			nvr := chFirstBundle.NameVersionRelease()
			first = fmt.Sprintf("%s (%s)", chFirstBundle.ID(), nvr.Version)
		}
		if chLastBundle != nil {
			nvr := chLastBundle.NameVersionRelease()
			last = fmt.Sprintf("%s (%s)", chLastBundle.ID(), nvr.Version)
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
			log.Printf("    %d successors from %s", successorCount, chFirstBundle.ID())
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
			log.Printf("Package-level: %d successors from %s in %s", count, firstBundle.ID(), time.Since(start))
		}
	}

	// Unpack a bundle image using its URI, parse as registry+v1, and render to plain manifests
	if err := unpackAndRenderBundle(ctx, composite, tmpDir); err != nil {
		return fmt.Errorf("unpack and render bundle: %w", err)
	}

	// GetPackage for unknown package (error case)
	_, err = cat.GetPackage(ctx, "this-package-does-not-exist-12345")
	if err != nil {
		log.Printf("Expected error for unknown package: %v", err)
	}

	log.Println("Done!")
	return nil
}

func findInterestingPackage(ctx context.Context, cat catalogv1.Catalog, pkgNames []string) (string, error) {
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

func unpackAndRenderBundle(ctx context.Context, composite catalogv1.CompositeUpdateGraph, tmpDir string) error {
	var bundle bundlev1.Bundle
	for b, err := range composite.ListBundles(ctx) {
		if err != nil {
			return fmt.Errorf("list bundles: %w", err)
		}
		if b.URI() != "" {
			bundle = b
			break
		}
	}
	if bundle == nil {
		log.Println("No bundle with URI found, skipping unpack")
		return nil
	}

	log.Printf("Unpacking bundle %s from %s...", bundle.ID(), bundle.URI())
	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}
	start := time.Now()
	if _, err := unpackImage(ctx, bundle.URI(), &imgbundle.RegistryV1Handler{}, bundleDir); err != nil {
		return fmt.Errorf("unpack bundle image: %w", err)
	}
	log.Printf("Unpacked bundle in %s", time.Since(start))

	regBundle, err := registryv1.FromFS(os.DirFS(bundleDir))
	if err != nil {
		return fmt.Errorf("parse registry+v1 bundle: %w", err)
	}
	log.Printf("Parsed registry+v1 bundle: %s (CSV: %s)", bundle.ID(), regBundle.CSV.GetName())

	objects, err := registryv1.ToPlainManifests(regBundle, "default")
	if err != nil {
		return fmt.Errorf("render plain manifests: %w", err)
	}
	log.Printf("Rendered %d plain manifests for namespace %q", len(objects), "default")
	for _, obj := range objects {
		log.Printf("  %s %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName())
	}
	return nil
}

func unpackImage(ctx context.Context, uri string, handler image.Handler, dest string) (string, error) {
	if !strings.HasPrefix(uri, dockerScheme) {
		return "", fmt.Errorf("unsupported URI scheme in %q (expected %s)", uri, dockerScheme)
	}
	imgRef, err := docker.ParseReference("//" + strings.TrimPrefix(uri, dockerScheme))
	if err != nil {
		return "", fmt.Errorf("parse reference %q: %w", uri, err)
	}

	client, err := image.NewContainersImageRepository(ctx, imgRef, &types.SystemContext{})
	if err != nil {
		return "", fmt.Errorf("create client: %w", err)
	}
	defer func() { _ = client.Close() }()

	desc, err := client.Resolve(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve: %w", err)
	}

	manifestBytes, _, err := client.FetchManifest(ctx, desc)
	if err != nil {
		return "", fmt.Errorf("fetch manifest: %w", err)
	}

	matches, err := handler.Matches(ctx, client, desc, manifestBytes)
	if err != nil {
		return "", fmt.Errorf("matches: %w", err)
	}
	if !matches {
		return "", fmt.Errorf("image %q does not match handler %q", uri, handler.Name())
	}

	return desc.Digest.String(), handler.Unpack(ctx, client, desc, manifestBytes, dest)
}
