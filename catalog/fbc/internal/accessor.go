package internal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
)

// PackageAccessor provides read-only access to a package's staging data.
type PackageAccessor struct {
	db          *sql.DB
	packageName string
}

func NewPackageAccessor(db *sql.DB, packageName string) *PackageAccessor {
	return &PackageAccessor{db: db, packageName: packageName}
}

func (a *PackageAccessor) Name() string { return a.packageName }

func (a *PackageAccessor) ExtData() (json.RawMessage, error) {
	var data nullRawMessage
	err := a.db.QueryRow("SELECT ext_data FROM "+TableRawPackage+" WHERE package_name = ?", a.packageName).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("querying ext_data for package %q: %w", a.packageName, err)
	}
	return json.RawMessage(data), nil
}

// Bundles pre-collects all bundles before yielding so that callers
// can safely nest other accessor queries (e.g., Channels) inside the
// loop. With MaxOpenConns(1), streaming would deadlock on nested queries.
func (a *PackageAccessor) Bundles() iter.Seq2[BundleAccessor, error] {
	return func(yield func(BundleAccessor, error) bool) {
		bundles, err := a.collectBundles()
		if err != nil {
			yield(nil, err)
			return
		}
		for i := range bundles {
			if !yield(&bundles[i], nil) {
				return
			}
		}
	}
}

// collectBundles returns all bundles for the package. Iteration order is unspecified.
func (a *PackageAccessor) collectBundles() ([]bundleAccessor, error) {
	rows, err := a.db.Query(
		"SELECT name, package_name, version, release, image, ext_data FROM "+TableRawBundle+" WHERE package_name = ?",
		a.packageName,
	)
	if err != nil {
		return nil, fmt.Errorf("querying bundles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bundles []bundleAccessor
	for rows.Next() {
		var b bundleAccessor
		if err := rows.Scan(&b.name, &b.pkg, &b.version, &b.release, &b.image, &b.extData); err != nil {
			return nil, err
		}
		bundles = append(bundles, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bundles, nil
}

// Channels pre-collects all channels before yielding so that callers
// can safely nest other accessor queries (e.g., Entries) inside the
// loop. With MaxOpenConns(1), streaming would deadlock on nested queries.
func (a *PackageAccessor) Channels() iter.Seq2[ChannelAccessor, error] {
	return func(yield func(ChannelAccessor, error) bool) {
		channels, err := a.collectChannels()
		if err != nil {
			yield(nil, err)
			return
		}
		for i := range channels {
			if !yield(&channels[i], nil) {
				return
			}
		}
	}
}

// collectChannels returns all channels for the package. Iteration order is unspecified.
func (a *PackageAccessor) collectChannels() ([]channelAccessor, error) {
	rows, err := a.db.Query(
		"SELECT name, ext_data FROM "+TableRawChannel+" WHERE package_name = ?",
		a.packageName,
	)
	if err != nil {
		return nil, fmt.Errorf("querying channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var channels []channelAccessor
	for rows.Next() {
		var ch channelAccessor
		if err := rows.Scan(&ch.name, &ch.extData); err != nil {
			return nil, err
		}
		ch.db = a.db
		ch.packageName = a.packageName
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

// Deprecations pre-collects all deprecations before yielding so that
// callers can safely nest other accessor queries inside the loop.
// With MaxOpenConns(1), streaming would deadlock on nested queries.
func (a *PackageAccessor) Deprecations() iter.Seq2[DeprecationAccessor, error] {
	return func(yield func(DeprecationAccessor, error) bool) {
		deprecations, err := a.collectDeprecations()
		if err != nil {
			yield(nil, err)
			return
		}
		for i := range deprecations {
			if !yield(&deprecations[i], nil) {
				return
			}
		}
	}
}

func (a *PackageAccessor) collectDeprecations() ([]deprecationAccessor, error) {
	rows, err := a.db.Query(
		"SELECT ext_data FROM "+TableRawDeprecation+" WHERE package_name = ?",
		a.packageName,
	)
	if err != nil {
		return nil, fmt.Errorf("querying deprecations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var deprecations []deprecationAccessor
	for rows.Next() {
		var d deprecationAccessor
		if err := rows.Scan(&d.extData); err != nil {
			return nil, err
		}
		deprecations = append(deprecations, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deprecations, nil
}

// Others pre-collects all "other" blobs before yielding so that
// callers can safely nest other accessor queries inside the loop.
// With MaxOpenConns(1), streaming would deadlock on nested queries.
func (a *PackageAccessor) Others() iter.Seq2[OtherAccessor, error] {
	return func(yield func(OtherAccessor, error) bool) {
		others, err := a.collectOthers()
		if err != nil {
			yield(nil, err)
			return
		}
		for i := range others {
			if !yield(&others[i], nil) {
				return
			}
		}
	}
}

func (a *PackageAccessor) collectOthers() ([]otherAccessor, error) {
	rows, err := a.db.Query(
		"SELECT schema, name, ext_data FROM "+TableRawOther+" WHERE package_name = ?",
		a.packageName,
	)
	if err != nil {
		return nil, fmt.Errorf("querying others: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var others []otherAccessor
	for rows.Next() {
		var o otherAccessor
		if err := rows.Scan(&o.schema, &o.name, &o.extData); err != nil {
			return nil, err
		}
		others = append(others, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return others, nil
}

// nullRawMessage handles scanning nullable JSON columns.
type nullRawMessage []byte

func (n *nullRawMessage) Scan(src any) error {
	if src == nil {
		*n = nil
		return nil
	}
	switch v := src.(type) {
	case string:
		*n = []byte(v)
	case []byte:
		*n = v
	default:
		return fmt.Errorf("unsupported type %T for nullRawMessage", src)
	}
	return nil
}

type BundleAccessor = interface {
	Name() string
	Package() string
	Version() string
	Release() string
	Image() string
	ExtData() json.RawMessage
}

type bundleAccessor struct {
	name    string
	pkg     string
	version string
	release string
	image   string
	extData nullRawMessage
}

func (b *bundleAccessor) Name() string             { return b.name }
func (b *bundleAccessor) Package() string          { return b.pkg }
func (b *bundleAccessor) Version() string          { return b.version }
func (b *bundleAccessor) Release() string          { return b.release }
func (b *bundleAccessor) Image() string            { return b.image }
func (b *bundleAccessor) ExtData() json.RawMessage { return json.RawMessage(b.extData) }

type ChannelAccessor = interface {
	Name() string
	Entries() iter.Seq2[ChannelEntryAccessor, error]
	ExtData() json.RawMessage
}

type channelAccessor struct {
	db          *sql.DB
	packageName string
	name        string
	extData     nullRawMessage
}

func (c *channelAccessor) Name() string             { return c.name }
func (c *channelAccessor) ExtData() json.RawMessage { return json.RawMessage(c.extData) }

// Entries pre-collects all channel entries before yielding so that
// callers can safely nest other accessor queries inside the loop.
// With MaxOpenConns(1), streaming would deadlock on nested queries.
func (c *channelAccessor) Entries() iter.Seq2[ChannelEntryAccessor, error] {
	return func(yield func(ChannelEntryAccessor, error) bool) {
		entries, err := c.collectEntries()
		if err != nil {
			yield(nil, err)
			return
		}
		for i := range entries {
			if !yield(&entries[i], nil) {
				return
			}
		}
	}
}

// collectEntries returns all entries for the channel. Iteration order is unspecified.
func (c *channelAccessor) collectEntries() ([]channelEntryAccessor, error) {
	rows, err := c.db.Query(
		"SELECT bundle_name, replaces, skips, skip_range FROM "+TableRawChannelEntry+" WHERE package_name = ? AND channel_name = ?",
		c.packageName, c.name,
	)
	if err != nil {
		return nil, fmt.Errorf("querying channel entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []channelEntryAccessor
	for rows.Next() {
		var e channelEntryAccessor
		var skipsStr string
		if err := rows.Scan(&e.bundleName, &e.replaces, &skipsStr, &e.skipRange); err != nil {
			return nil, err
		}
		if skipsStr != "" {
			e.skips = strings.Split(skipsStr, ",")
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

type ChannelEntryAccessor = interface {
	BundleName() string
	Replaces() string
	Skips() []string
	SkipRange() string
}

type channelEntryAccessor struct {
	bundleName string
	replaces   string
	skips      []string
	skipRange  string
}

func (e *channelEntryAccessor) BundleName() string { return e.bundleName }
func (e *channelEntryAccessor) Replaces() string   { return e.replaces }
func (e *channelEntryAccessor) Skips() []string    { return e.skips }
func (e *channelEntryAccessor) SkipRange() string  { return e.skipRange }

type DeprecationAccessor = interface {
	ExtData() json.RawMessage
}

type deprecationAccessor struct {
	extData nullRawMessage
}

func (d *deprecationAccessor) ExtData() json.RawMessage { return json.RawMessage(d.extData) }

type OtherAccessor = interface {
	Schema() string
	Name() string
	ExtData() json.RawMessage
}

type otherAccessor struct {
	schema  string
	name    string
	extData nullRawMessage
}

func (o *otherAccessor) Schema() string           { return o.schema }
func (o *otherAccessor) Name() string             { return o.name }
func (o *otherAccessor) ExtData() json.RawMessage { return json.RawMessage(o.extData) }
