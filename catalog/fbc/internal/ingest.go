package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
)

// Ingest walks fsys via declcfg.WalkMetasFS, parses each blob, and
// inserts structured fields into raw tables. A dedicated writer
// goroutine batches inserts into transactions for performance.
type ingestRow struct {
	fn func(tx *sql.Tx) error
}

func Ingest(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	rowCh := make(chan ingestRow, 256)
	errCh := make(chan error, 1)

	go func() {
		errCh <- batchWriter(ctx, db, rowCh)
	}()

	walkErr := declcfg.WalkMetasFS(ctx, fsys, func(_ string, meta *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}
		insert, err := metaToInsert(meta)
		if err != nil {
			return err
		}
		if insert == nil {
			return nil
		}
		select {
		case rowCh <- ingestRow{fn: insert}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	close(rowCh)
	writerErr := <-errCh

	if walkErr != nil {
		return walkErr
	}
	return writerErr
}

func metaToInsert(meta *declcfg.Meta) (func(tx *sql.Tx) error, error) {
	switch meta.Schema {
	case declcfg.SchemaPackage:
		var p declcfg.Package
		if err := json.Unmarshal(meta.Blob, &p); err != nil {
			return nil, fmt.Errorf("parse package: %w", err)
		}
		return func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO raw_olm_package (name) VALUES (?)", p.Name)
			return err
		}, nil

	case declcfg.SchemaChannel:
		var ch declcfg.Channel
		if err := json.Unmarshal(meta.Blob, &ch); err != nil {
			return nil, fmt.Errorf("parse channel: %w", err)
		}
		return func(tx *sql.Tx) error {
			if _, err := tx.Exec("INSERT INTO raw_olm_channel (name, package_name) VALUES (?, ?)",
				ch.Name, ch.Package); err != nil {
				return err
			}
			for _, entry := range ch.Entries {
				skips := strings.Join(entry.Skips, ",")
				if _, err := tx.Exec(
					"INSERT INTO raw_olm_channel_entry (channel_name, package_name, bundle_name, replaces, skips, skip_range) VALUES (?, ?, ?, ?, ?, ?)",
					ch.Name, ch.Package, entry.Name, entry.Replaces, skips, entry.SkipRange,
				); err != nil {
					return err
				}
			}
			return nil
		}, nil

	case declcfg.SchemaBundle:
		var b declcfg.Bundle
		if err := json.Unmarshal(meta.Blob, &b); err != nil {
			return nil, fmt.Errorf("parse bundle: %w", err)
		}
		version, err := extractBundleVersion(b)
		if err != nil {
			return nil, err
		}
		return func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO raw_olm_bundle (name, package_name, version) VALUES (?, ?, ?)",
				b.Name, b.Package, version)
			return err
		}, nil

	default:
		return nil, nil
	}
}

func extractBundleVersion(b declcfg.Bundle) (string, error) {
	for _, prop := range b.Properties {
		if prop.Type != property.TypePackage {
			continue
		}
		var pkg property.Package
		if err := json.Unmarshal(prop.Value, &pkg); err != nil {
			return "", fmt.Errorf("parse %s property for bundle %q: %w", property.TypePackage, b.Name, err)
		}
		return pkg.Version, nil
	}
	return "", fmt.Errorf("bundle %q has no %s property", b.Name, property.TypePackage)
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
				_ = tx.Rollback()
				return err
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
