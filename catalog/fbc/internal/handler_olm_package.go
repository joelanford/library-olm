package internal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	bsemver "github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"go.podman.io/image/v5/docker/reference"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

type OLMPackageHandler struct{}

func (h *OLMPackageHandler) Schema() string { return declcfg.SchemaPackage }

type validatedBundle struct {
	name    string
	pkg     string
	version string
	release string
	uri     string
}

type channelEntry struct {
	name      string
	replaces  string
	skips     []string
	skipRange string
}

func (h *OLMPackageHandler) Normalize(ctx context.Context, rawDB *sql.DB, w catalogv1.Writer, packageName string) error {
	pkg := NewPackageAccessor(rawDB, packageName)

	bundles, err := h.validateBundles(pkg)
	if err != nil {
		return fmt.Errorf("validate bundles for %q: %w", packageName, err)
	}

	var channels []channelData
	for ch, err := range pkg.Channels() {
		if err != nil {
			return fmt.Errorf("read channels for %q: %w", packageName, err)
		}
		var entries []channelEntry
		for entry, err := range ch.Entries() {
			if err != nil {
				return fmt.Errorf("read channel entries for %q/%q: %w", packageName, ch.Name(), err)
			}
			entries = append(entries, channelEntry{
				name:      entry.BundleName(),
				replaces:  entry.Replaces(),
				skips:     entry.Skips(),
				skipRange: entry.SkipRange(),
			})
		}
		channels = append(channels, channelData{name: ch.Name(), entries: entries})
	}

	if err := h.validateEntries(packageName, bundles, channels); err != nil {
		return err
	}

	for _, ch := range channels {
		for _, e := range ch.entries {
			if e.skipRange != "" {
				if _, parseErr := bsemver.ParseRange(e.skipRange); parseErr != nil {
					return fmt.Errorf("channel %q: skipRange for %q: parse skipRange %q: %w", ch.name, e.name, e.skipRange, parseErr)
				}
			}
		}
	}

	pkgPath := []string{packageName}
	if err := w.CreateGraph(pkgPath); err != nil {
		return fmt.Errorf("insert package graph %q: %w", packageName, err)
	}

	for _, b := range bundles {
		if err := w.InsertBundle(b.name, b.pkg, b.version, b.release, b.uri); err != nil {
			return fmt.Errorf("insert bundle %q: %w", b.name, err)
		}
	}

	for _, ch := range channels {
		chPath := []string{packageName, ch.name}
		if err := w.CreateGraph(chPath); err != nil {
			return fmt.Errorf("insert graph for channel %q: %w", ch.name, err)
		}

		for _, e := range ch.entries {
			if err := w.AddBundleToGraph(chPath, e.name); err != nil {
				return fmt.Errorf("add bundle %q to channel %q: %w", e.name, ch.name, err)
			}
		}

		if err := h.writeChannelSuccessors(w, chPath, ch.entries); err != nil {
			return fmt.Errorf("compute successors for channel %q: %w", ch.name, err)
		}
	}

	return nil
}

func (h *OLMPackageHandler) validateBundles(pkg *PackageAccessor) ([]validatedBundle, error) {
	var bundles []validatedBundle
	for b, err := range pkg.Bundles() {
		if err != nil {
			return nil, err
		}
		if _, err := bsemver.Parse(b.Version()); err != nil {
			return nil, fmt.Errorf("parse version %q for bundle %q: %w", b.Version(), b.Name(), err)
		}
		if b.Release() != "" {
			if _, err := bundlev1.ParseRelease(b.Release()); err != nil {
				return nil, fmt.Errorf("parse release %q for bundle %q: %w", b.Release(), b.Name(), err)
			}
		}
		if b.Image() == "" {
			return nil, fmt.Errorf("bundle %q has no image", b.Name())
		}
		ref, err := reference.ParseNamed(b.Image())
		if err != nil {
			return nil, fmt.Errorf("parse image %q for bundle %q: %w", b.Image(), b.Name(), err)
		}
		if _, ok := ref.(reference.NamedTagged); !ok {
			if _, ok := ref.(reference.Canonical); !ok {
				return nil, fmt.Errorf("image %q for bundle %q must be tagged or canonical", b.Image(), b.Name())
			}
		}
		bundles = append(bundles, validatedBundle{
			name:    b.Name(),
			pkg:     b.Package(),
			version: b.Version(),
			release: b.Release(),
			uri:     "docker://" + ref.String(),
		})
	}
	return bundles, nil
}

type channelData struct {
	name    string
	entries []channelEntry
}

func (h *OLMPackageHandler) validateEntries(packageName string, bundles []validatedBundle, channels []channelData) error {
	bundleNames := make(map[string]bool, len(bundles))
	for _, b := range bundles {
		bundleNames[b.name] = true
	}

	var missing []string
	for _, ch := range channels {
		for _, e := range ch.entries {
			if !bundleNames[e.name] {
				missing = append(missing, fmt.Sprintf("channel %q entry %q", ch.name, e.name))
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("validate package %q: channel entries reference unknown bundles: %s", packageName, strings.Join(missing, ", "))
	}
	return nil
}

func (h *OLMPackageHandler) writeChannelSuccessors(w catalogv1.Writer, chPath []string, entries []channelEntry) error {
	for _, e := range entries {
		if e.replaces != "" {
			if err := w.InsertBundle(e.replaces, "", "", "", ""); err != nil {
				return err
			}
			if err := w.AddEdge(chPath, e.replaces, e.name); err != nil {
				return err
			}
		}

		for _, skip := range e.skips {
			if err := w.InsertBundle(skip, "", "", "", ""); err != nil {
				return err
			}
			if err := w.AddEdge(chPath, skip, e.name); err != nil {
				return err
			}
		}

		if e.skipRange != "" {
			if err := w.AddPredecessorRange(chPath, e.name, e.skipRange); err != nil {
				return fmt.Errorf("skipRange for %q: %w", e.name, err)
			}
		}
	}
	return nil
}
