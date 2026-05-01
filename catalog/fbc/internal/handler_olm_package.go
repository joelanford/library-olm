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
)

type OLMPackageHandler struct{}

func (h *OLMPackageHandler) Schema() string { return declcfg.SchemaPackage }

func (h *OLMPackageHandler) Normalize(ctx context.Context, tx *sql.Tx, packageName string) error {
	if err := h.validate(tx, packageName); err != nil {
		return fmt.Errorf("validate package %q: %w", packageName, err)
	}

	pkgGraphID, err := h.insertPackageGraph(tx, packageName)
	if err != nil {
		return fmt.Errorf("insert package graph %q: %w", packageName, err)
	}

	if err := h.insertBundles(tx, packageName); err != nil {
		return fmt.Errorf("insert bundles for %q: %w", packageName, err)
	}

	if err := h.insertChannelGraphsAndEntries(tx, packageName, pkgGraphID); err != nil {
		return fmt.Errorf("insert channel graphs for %q: %w", packageName, err)
	}

	if err := h.computeSuccessors(tx, packageName, pkgGraphID); err != nil {
		return fmt.Errorf("compute successors for %q: %w", packageName, err)
	}

	return nil
}

func (h *OLMPackageHandler) validate(tx *sql.Tx, packageName string) error {
	rows, err := tx.Query(`
		SELECT ce.channel_name, ce.bundle_name
		FROM raw_olm_channel_entry ce
		WHERE ce.package_name = ?
		  AND NOT EXISTS (
		    SELECT 1 FROM raw_olm_bundle b
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

func (h *OLMPackageHandler) insertPackageGraph(tx *sql.Tx, packageName string) (int64, error) {
	res, err := tx.Exec("INSERT INTO graphs (name, parent_id) VALUES (?, NULL)", packageName)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (h *OLMPackageHandler) insertBundles(tx *sql.Tx, packageName string) error {
	rows, err := tx.Query("SELECT name, version, release, image FROM raw_olm_bundle WHERE package_name = ?", packageName)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name, versionStr, releaseStr, image string
		if err := rows.Scan(&name, &versionStr, &releaseStr, &image); err != nil {
			return err
		}
		if _, err := bsemver.Parse(versionStr); err != nil {
			return fmt.Errorf("parse version %q for bundle %q: %w", versionStr, name, err)
		}
		if releaseStr != "" {
			if _, err := bundlev1.ParseRelease(releaseStr); err != nil {
				return fmt.Errorf("parse release %q for bundle %q: %w", releaseStr, name, err)
			}
		}
		if image == "" {
			return fmt.Errorf("bundle %q has no image", name)
		}
		ref, err := reference.ParseNamed(image)
		if err != nil {
			return fmt.Errorf("parse image %q for bundle %q: %w", image, name, err)
		}
		if _, ok := ref.(reference.NamedTagged); !ok {
			if _, ok := ref.(reference.Canonical); !ok {
				return fmt.Errorf("image %q for bundle %q must be tagged or canonical", image, name)
			}
		}
		uri := "docker://" + ref.String()
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO bundles (id, package_name, version, release, uri) VALUES (?, ?, ?, ?, ?)",
			name, packageName, versionStr, releaseStr, uri,
		); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (h *OLMPackageHandler) insertChannelGraphsAndEntries(tx *sql.Tx, packageName string, pkgGraphID int64) error {
	chRows, err := tx.Query("SELECT name FROM raw_olm_channel WHERE package_name = ?", packageName)
	if err != nil {
		return err
	}
	defer func() { _ = chRows.Close() }()

	var channels []string
	for chRows.Next() {
		var name string
		if err := chRows.Scan(&name); err != nil {
			return err
		}
		channels = append(channels, name)
	}
	if err := chRows.Err(); err != nil {
		return err
	}

	for _, chName := range channels {
		res, err := tx.Exec("INSERT INTO graphs (name, parent_id) VALUES (?, ?)", chName, pkgGraphID)
		if err != nil {
			return fmt.Errorf("insert graph for channel %q: %w", chName, err)
		}
		chGraphID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(`
			INSERT INTO graph_bundles (graph_id, bundle_id)
			SELECT ?, ce.bundle_name
			FROM raw_olm_channel_entry ce
			WHERE ce.package_name = ? AND ce.channel_name = ?`,
			chGraphID, packageName, chName); err != nil {
			return fmt.Errorf("insert graph_bundles for channel %q: %w", chName, err)
		}
	}
	return nil
}

func (h *OLMPackageHandler) computeSuccessors(tx *sql.Tx, packageName string, pkgGraphID int64) error {
	chRows, err := tx.Query("SELECT id, name FROM graphs WHERE parent_id = ?", pkgGraphID)
	if err != nil {
		return err
	}
	defer func() { _ = chRows.Close() }()

	type channelInfo struct {
		id   int64
		name string
	}
	var channels []channelInfo
	for chRows.Next() {
		var ci channelInfo
		if err := chRows.Scan(&ci.id, &ci.name); err != nil {
			return err
		}
		channels = append(channels, ci)
	}
	if err := chRows.Err(); err != nil {
		return err
	}

	for _, ch := range channels {
		if err := h.computeChannelSuccessors(tx, packageName, ch.id, ch.name); err != nil {
			return fmt.Errorf("compute successors for channel %q: %w", ch.name, err)
		}
	}
	return nil
}

func (h *OLMPackageHandler) computeChannelSuccessors(tx *sql.Tx, packageName string, chGraphID int64, chName string) error {
	entryRows, err := tx.Query(`
		SELECT ce.bundle_name, ce.replaces, ce.skips, ce.skip_range
		FROM raw_olm_channel_entry ce
		WHERE ce.package_name = ? AND ce.channel_name = ?`, packageName, chName)
	if err != nil {
		return err
	}
	defer func() { _ = entryRows.Close() }()

	type entry struct {
		name      string
		replaces  string
		skips     []string
		skipRange string
	}
	var entries []entry
	for entryRows.Next() {
		var e entry
		var skipsStr, skipRange string
		if err := entryRows.Scan(&e.name, &e.replaces, &skipsStr, &skipRange); err != nil {
			return err
		}
		if skipsStr != "" {
			e.skips = strings.Split(skipsStr, ",")
		}
		e.skipRange = skipRange
		entries = append(entries, e)
	}
	if err := entryRows.Err(); err != nil {
		return err
	}

	for _, e := range entries {
		// replaces edge: from the replaced bundle to this one
		if e.replaces != "" {
			if err := h.ensurePhantomBundle(tx, e.replaces); err != nil {
				return err
			}
			if err := h.insertSuccessor(tx, chGraphID, e.replaces, e.name); err != nil {
				return err
			}
		}

		// skips edges: from each skipped bundle to this one
		for _, skip := range e.skips {
			if err := h.ensurePhantomBundle(tx, skip); err != nil {
				return err
			}
			if err := h.insertSuccessor(tx, chGraphID, skip, e.name); err != nil {
				return err
			}
		}

		// skipRange edges: from every bundle in the channel whose version matches the range
		if e.skipRange != "" {
			if err := h.computeSkipRangeSuccessors(tx, chGraphID, e.name, e.skipRange); err != nil {
				return fmt.Errorf("skipRange for %q: %w", e.name, err)
			}
		}
	}
	return nil
}

func (h *OLMPackageHandler) ensurePhantomBundle(tx *sql.Tx, bundleName string) error {
	_, err := tx.Exec("INSERT OR IGNORE INTO bundles (id) VALUES (?)", bundleName)
	return err
}

func (h *OLMPackageHandler) computeSkipRangeSuccessors(tx *sql.Tx, chGraphID int64, bundleName string, skipRangeStr string) error {
	rng, err := bsemver.ParseRange(skipRangeStr)
	if err != nil {
		return fmt.Errorf("parse skipRange %q: %w", skipRangeStr, err)
	}

	rows, err := tx.Query(`
		SELECT b.id, b.version
		FROM graph_bundles gb
		JOIN bundles b ON b.id = gb.bundle_id
		WHERE gb.graph_id = ? AND b.id != ? AND b.version != ''`, chGraphID, bundleName)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name, versionStr string
		if err := rows.Scan(&name, &versionStr); err != nil {
			return err
		}
		ver, err := bsemver.Parse(versionStr)
		if err != nil {
			continue
		}
		if rng(ver) {
			if err := h.insertSuccessor(tx, chGraphID, name, bundleName); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func (h *OLMPackageHandler) insertSuccessor(tx *sql.Tx, graphID int64, fromBundleID, toBundleID string) error {
	_, err := tx.Exec(
		"INSERT OR IGNORE INTO successors (graph_id, from_bundle_id, to_bundle_id) VALUES (?, ?, ?)",
		graphID, fromBundleID, toBundleID)
	return err
}
