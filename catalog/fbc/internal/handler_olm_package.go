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

// validatedBundle holds bundle data that has passed validation and is ready to write.
type validatedBundle struct {
	name    string
	pkg     string
	version string
	release string
	uri     string
}

func (h *OLMPackageHandler) Normalize(ctx context.Context, rawDB *sql.DB, w catalogv1.Writer, packageName string) error {
	// Phase 1: Validate everything by reading raw tables.
	if err := h.validate(rawDB, packageName); err != nil {
		return fmt.Errorf("validate package %q: %w", packageName, err)
	}

	bundles, err := h.validateBundles(rawDB, packageName)
	if err != nil {
		return fmt.Errorf("validate bundles for %q: %w", packageName, err)
	}

	channels, err := h.readChannels(rawDB, packageName)
	if err != nil {
		return fmt.Errorf("read channels for %q: %w", packageName, err)
	}

	channelEntries, err := h.readAllChannelEntries(rawDB, packageName, channels)
	if err != nil {
		return fmt.Errorf("read channel entries for %q: %w", packageName, err)
	}

	// Validate skipRanges before writing anything.
	for chName, entries := range channelEntries {
		for _, e := range entries {
			if e.skipRange != "" {
				if _, parseErr := bsemver.ParseRange(e.skipRange); parseErr != nil {
					return fmt.Errorf("channel %q: skipRange for %q: parse skipRange %q: %w", chName, e.name, e.skipRange, parseErr)
				}
			}
		}
	}

	// Phase 2: All validation passed. Write everything.
	pkgGraphID, err := w.CreateGraph(packageName, nil)
	if err != nil {
		return fmt.Errorf("insert package graph %q: %w", packageName, err)
	}

	for _, b := range bundles {
		if err := w.InsertBundle(b.name, b.pkg, b.version, b.release, b.uri); err != nil {
			return fmt.Errorf("insert bundle %q: %w", b.name, err)
		}
	}

	for _, chName := range channels {
		chGraphID, err := w.CreateGraph(chName, &pkgGraphID)
		if err != nil {
			return fmt.Errorf("insert graph for channel %q: %w", chName, err)
		}

		entries := channelEntries[chName]
		for _, e := range entries {
			if err := w.AddBundleToGraph(chGraphID, e.name); err != nil {
				return fmt.Errorf("add bundle %q to channel %q: %w", e.name, chName, err)
			}
		}

		if err := h.writeChannelSuccessors(w, chGraphID, entries); err != nil {
			return fmt.Errorf("compute successors for channel %q: %w", chName, err)
		}
	}

	return nil
}

func (h *OLMPackageHandler) validate(rawDB *sql.DB, packageName string) error {
	rows, err := rawDB.Query(`
		SELECT ce.channel_name, ce.bundle_name
		FROM `+TableRawChannelEntry+` ce
		WHERE ce.package_name = ?
		  AND NOT EXISTS (
		    SELECT 1 FROM `+TableRawBundle+` b
		    WHERE b.package_name = ce.package_name AND b.name = ce.bundle_name
		  )`, packageName)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var missing []string
	for rows.Next() {
		var chName, bName string
		if err := rows.Scan(&chName, &bName); err != nil {
			return err
		}
		missing = append(missing, fmt.Sprintf("channel %q entry %q", chName, bName))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("channel entries reference unknown bundles: %s", strings.Join(missing, ", "))
	}
	return nil
}

// validateBundles reads all bundles for a package from raw tables, validates
// each one, and returns the validated data ready for writing.
func (h *OLMPackageHandler) validateBundles(rawDB *sql.DB, packageName string) ([]validatedBundle, error) {
	rows, err := rawDB.Query("SELECT name, version, release, image FROM "+TableRawBundle+" WHERE package_name = ?", packageName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var bundles []validatedBundle
	for rows.Next() {
		var name, versionStr, releaseStr, image string
		if err := rows.Scan(&name, &versionStr, &releaseStr, &image); err != nil {
			return nil, err
		}
		if _, err := bsemver.Parse(versionStr); err != nil {
			return nil, fmt.Errorf("parse version %q for bundle %q: %w", versionStr, name, err)
		}
		if releaseStr != "" {
			if _, err := bundlev1.ParseRelease(releaseStr); err != nil {
				return nil, fmt.Errorf("parse release %q for bundle %q: %w", releaseStr, name, err)
			}
		}
		if image == "" {
			return nil, fmt.Errorf("bundle %q has no image", name)
		}
		ref, err := reference.ParseNamed(image)
		if err != nil {
			return nil, fmt.Errorf("parse image %q for bundle %q: %w", image, name, err)
		}
		if _, ok := ref.(reference.NamedTagged); !ok {
			if _, ok := ref.(reference.Canonical); !ok {
				return nil, fmt.Errorf("image %q for bundle %q must be tagged or canonical", image, name)
			}
		}
		bundles = append(bundles, validatedBundle{
			name:    name,
			pkg:     packageName,
			version: versionStr,
			release: releaseStr,
			uri:     "docker://" + ref.String(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bundles, nil
}

func (h *OLMPackageHandler) readChannels(rawDB *sql.DB, packageName string) ([]string, error) {
	rows, err := rawDB.Query("SELECT name FROM "+TableRawChannel+" WHERE package_name = ?", packageName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var channels []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		channels = append(channels, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

type channelEntry struct {
	name      string
	replaces  string
	skips     []string
	skipRange string
}

func (h *OLMPackageHandler) readAllChannelEntries(rawDB *sql.DB, packageName string, channels []string) (map[string][]channelEntry, error) {
	result := make(map[string][]channelEntry, len(channels))
	for _, chName := range channels {
		entries, err := h.readChannelEntries(rawDB, packageName, chName)
		if err != nil {
			return nil, fmt.Errorf("channel %q: %w", chName, err)
		}
		result[chName] = entries
	}
	return result, nil
}

func (h *OLMPackageHandler) readChannelEntries(rawDB *sql.DB, packageName, chName string) ([]channelEntry, error) {
	rows, err := rawDB.Query(`
		SELECT ce.bundle_name, ce.replaces, ce.skips, ce.skip_range
		FROM `+TableRawChannelEntry+` ce
		WHERE ce.package_name = ? AND ce.channel_name = ?`, packageName, chName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []channelEntry
	for rows.Next() {
		var e channelEntry
		var skipsStr, skipRange string
		if err := rows.Scan(&e.name, &e.replaces, &skipsStr, &skipRange); err != nil {
			return nil, err
		}
		if skipsStr != "" {
			e.skips = strings.Split(skipsStr, ",")
		}
		e.skipRange = skipRange
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (h *OLMPackageHandler) writeChannelSuccessors(w catalogv1.Writer, chGraphID catalogv1.GraphID, entries []channelEntry) error {
	for _, e := range entries {
		// replaces edge: from the replaced bundle to this one
		if e.replaces != "" {
			// Ensure phantom bundle exists (INSERT OR IGNORE via Writer)
			if err := w.InsertBundle(e.replaces, "", "", "", ""); err != nil {
				return err
			}
			if err := w.AddEdge(chGraphID, e.replaces, e.name); err != nil {
				return err
			}
		}

		// skips edges: from each skipped bundle to this one
		for _, skip := range e.skips {
			if err := w.InsertBundle(skip, "", "", "", ""); err != nil {
				return err
			}
			if err := w.AddEdge(chGraphID, skip, e.name); err != nil {
				return err
			}
		}

		// skipRange: store as a predecessor range, evaluated at query time
		if e.skipRange != "" {
			if err := w.AddPredecessorRange(chGraphID, e.name, e.skipRange); err != nil {
				return fmt.Errorf("skipRange for %q: %w", e.name, err)
			}
		}
	}
	return nil
}
