package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
)

type ingestRow struct {
	fn func(tx *sql.Tx) error
}

type IngestResult struct {
	PackageErrors map[string][]error
}

func Ingest(ctx context.Context, db *sql.DB, fsys fs.FS) (*IngestResult, error) {
	rowCh := make(chan ingestRow, 256)
	errCh := make(chan error, 1)

	go func() {
		errCh <- batchWriter(ctx, db, rowCh)
	}()

	var mu sync.Mutex
	pkgErrors := make(map[string][]error)

	recordError := func(pkg string, err error) {
		mu.Lock()
		pkgErrors[pkg] = append(pkgErrors[pkg], err)
		mu.Unlock()
	}

	walkErr := declcfg.WalkMetasFS(ctx, fsys, func(_ string, meta *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}

		var insert func(tx *sql.Tx) error
		var parseErr error
		switch meta.Schema {
		case declcfg.SchemaPackage:
			insert, parseErr = parsePackage(meta)
		case declcfg.SchemaChannel:
			insert, parseErr = parseChannel(meta)
		case declcfg.SchemaBundle:
			insert, parseErr = parseBundle(meta)
		default:
			return nil
		}
		if parseErr != nil {
			if meta.Package == "" {
				return parseErr
			}
			recordError(meta.Package, parseErr)
			return nil
		}
		return sendRow(ctx, rowCh, insert)
	})

	close(rowCh)
	writerErr := <-errCh

	if walkErr != nil {
		return nil, walkErr
	}
	if writerErr != nil {
		return nil, writerErr
	}
	return &IngestResult{PackageErrors: pkgErrors}, nil
}

func sendRow(ctx context.Context, rowCh chan<- ingestRow, fn func(tx *sql.Tx) error) error {
	select {
	case rowCh <- ingestRow{fn: fn}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func parsePackage(meta *declcfg.Meta) (func(tx *sql.Tx) error, error) {
	var p declcfg.Package
	if err := json.Unmarshal(meta.Blob, &p); err != nil {
		return nil, fmt.Errorf("parse package: %w", err)
	}
	return func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO "+TableRawPackage+" (package_name) VALUES (?)", p.Name)
		return err
	}, nil
}

func parseChannel(meta *declcfg.Meta) (func(tx *sql.Tx) error, error) {
	var ch declcfg.Channel
	if err := json.Unmarshal(meta.Blob, &ch); err != nil {
		return nil, fmt.Errorf("parse channel: %w", err)
	}
	return func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO "+TableRawChannel+" (name, package_name) VALUES (?, ?)",
			ch.Name, ch.Package); err != nil {
			return err
		}
		for _, entry := range ch.Entries {
			skips := strings.Join(entry.Skips, ",")
			if _, err := tx.Exec(
				"INSERT INTO "+TableRawChannelEntry+" (channel_name, package_name, bundle_name, replaces, skips, skip_range) VALUES (?, ?, ?, ?, ?, ?)",
				ch.Name, ch.Package, entry.Name, entry.Replaces, skips, entry.SkipRange,
			); err != nil {
				return err
			}
		}
		return nil
	}, nil
}

func parseBundle(meta *declcfg.Meta) (func(tx *sql.Tx) error, error) {
	var b declcfg.Bundle
	if err := json.Unmarshal(meta.Blob, &b); err != nil {
		return nil, fmt.Errorf("parse bundle: %w", err)
	}
	version, release, err := extractBundleVersionRelease(b)
	if err != nil {
		return nil, err
	}
	return func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO "+TableRawBundle+" (name, package_name, version, release, image) VALUES (?, ?, ?, ?, ?)",
			b.Name, b.Package, version, release, b.Image)
		return err
	}, nil
}

func extractBundleVersionRelease(b declcfg.Bundle) (string, string, error) {
	for _, prop := range b.Properties {
		if prop.Type != property.TypePackage {
			continue
		}
		var pkg property.Package
		if err := json.Unmarshal(prop.Value, &pkg); err != nil {
			return "", "", fmt.Errorf("parse %s property for bundle %q: %w", property.TypePackage, b.Name, err)
		}
		return pkg.Version, pkg.Release, nil
	}
	return "", "", fmt.Errorf("bundle %q has no %s property", b.Name, property.TypePackage)
}

const batchSize = 500

func batchWriter(ctx context.Context, db *sql.DB, rowCh <-chan ingestRow) error {
	var batch []func(tx *sql.Tx) error
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		for _, fn := range batch {
			if err := fn(tx); err != nil {
				return errors.Join(err, tx.Rollback())
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit transaction: %w", err)
		}
		batch = batch[:0]
		return nil
	}

	for r := range rowCh {
		batch = append(batch, r.fn)
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}
