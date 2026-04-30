package internal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

type OLMPackageHandler struct{}

func (h *OLMPackageHandler) Schema() string { return declcfg.SchemaPackage }

func (h *OLMPackageHandler) CompanionSchemas() []string {
	return []string{declcfg.SchemaChannel, declcfg.SchemaBundle}
}

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
	// Check channels reference this package (enforced by PK, but verify entries)
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
	defer rows.Close()

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

	// Check replaces targets exist as bundles
	replRows, err := tx.Query(`
		SELECT ce.channel_name, ce.bundle_name, ce.replaces
		FROM raw_olm_channel_entry ce
		WHERE ce.package_name = ? AND ce.replaces != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM raw_olm_bundle b
		    WHERE b.package_name = ce.package_name AND b.name = ce.replaces
		  )`, packageName)
	if err != nil {
		return err
	}
	defer replRows.Close()

	var badReplaces []string
	for replRows.Next() {
		var chName, bName, replaces string
		if err := replRows.Scan(&chName, &bName, &replaces); err != nil {
			return err
		}
		badReplaces = append(badReplaces, fmt.Sprintf("channel %q: %q replaces unknown %q", chName, bName, replaces))
	}
	if err := replRows.Err(); err != nil {
		return err
	}
	if len(badReplaces) > 0 {
		return fmt.Errorf("replaces references unknown bundles: %s", strings.Join(badReplaces, ", "))
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
	rows, err := tx.Query("SELECT name, version FROM raw_olm_bundle WHERE package_name = ?", packageName)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, versionStr string
		if err := rows.Scan(&name, &versionStr); err != nil {
			return err
		}
		ver, err := semver.Parse(versionStr)
		if err != nil {
			return fmt.Errorf("parse version %q for bundle %q: %w", versionStr, name, err)
		}
		releaseStr := ""
		if len(ver.Pre) > 0 {
			parts := make([]string, len(ver.Pre))
			for i, p := range ver.Pre {
				parts[i] = p.String()
			}
			releaseStr = strings.Join(parts, ".")
		}
		versionOnly := semver.Version{Major: ver.Major, Minor: ver.Minor, Patch: ver.Patch}
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO bundles (name, version, release) VALUES (?, ?, ?)",
			name, versionOnly.String(), releaseStr,
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
	defer chRows.Close()

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
			SELECT ?, b.id
			FROM raw_olm_channel_entry ce
			JOIN bundles b ON b.name = ce.bundle_name
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
	defer chRows.Close()

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
	defer entryRows.Close()

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
		toBundleID, err := h.lookupBundleID(tx, e.name)
		if err != nil {
			return fmt.Errorf("lookup bundle %q: %w", e.name, err)
		}

		// replaces edge
		if e.replaces != "" {
			fromBundleID, err := h.lookupBundleID(tx, e.replaces)
			if err != nil {
				return fmt.Errorf("lookup replaces target %q: %w", e.replaces, err)
			}
			if err := h.insertSuccessor(tx, chGraphID, fromBundleID, toBundleID); err != nil {
				return err
			}
		}

		// skips edges
		for _, skip := range e.skips {
			fromBundleID, err := h.lookupBundleID(tx, skip)
			if err != nil {
				return fmt.Errorf("lookup skips target %q: %w", skip, err)
			}
			if err := h.insertSuccessor(tx, chGraphID, fromBundleID, toBundleID); err != nil {
				return err
			}
		}

		// skipRange edges
		if e.skipRange != "" {
			if err := h.computeSkipRangeSuccessors(tx, chGraphID, e.name, toBundleID, e.skipRange); err != nil {
				return fmt.Errorf("skipRange for %q: %w", e.name, err)
			}
		}
	}
	return nil
}

func (h *OLMPackageHandler) computeSkipRangeSuccessors(tx *sql.Tx, chGraphID int64, bundleName string, toBundleID int64, skipRangeStr string) error {
	rng, err := semver.ParseRange(skipRangeStr)
	if err != nil {
		return fmt.Errorf("parse skipRange %q: %w", skipRangeStr, err)
	}

	// Get all bundles in this channel with their versions
	rows, err := tx.Query(`
		SELECT b.id, b.version, b.release
		FROM graph_bundles gb
		JOIN bundles b ON b.id = gb.bundle_id
		WHERE gb.graph_id = ? AND b.name != ?`, chGraphID, bundleName)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var fromID int64
		var versionStr, releaseStr string
		if err := rows.Scan(&fromID, &versionStr, &releaseStr); err != nil {
			return err
		}
		ver, err := semver.Parse(versionStr)
		if err != nil {
			continue
		}
		if releaseStr != "" {
			parts := strings.Split(releaseStr, ".")
			pre := make([]semver.PRVersion, len(parts))
			for i, p := range parts {
				pr, err := semver.NewPRVersion(p)
				if err != nil {
					continue
				}
				pre[i] = pr
			}
			ver.Pre = pre
		}
		if rng(ver) {
			if err := h.insertSuccessor(tx, chGraphID, fromID, toBundleID); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func (h *OLMPackageHandler) lookupBundleID(tx *sql.Tx, bundleName string) (int64, error) {
	var id int64
	err := tx.QueryRow("SELECT id FROM bundles WHERE name = ?", bundleName).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("bundle %q not found in normalized bundles table", bundleName)
	}
	return id, nil
}

func (h *OLMPackageHandler) insertSuccessor(tx *sql.Tx, graphID, fromBundleID, toBundleID int64) error {
	_, err := tx.Exec(
		"INSERT OR IGNORE INTO successors (graph_id, from_bundle_id, to_bundle_id) VALUES (?, ?, ?)",
		graphID, fromBundleID, toBundleID)
	return err
}
