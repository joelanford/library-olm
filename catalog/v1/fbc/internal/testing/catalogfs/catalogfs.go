package catalogfs

import (
	"encoding/json"
	"fmt"
	"testing/fstest"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

// CatalogFSBuilder constructs an FBC catalog filesystem.
type CatalogFSBuilder interface {
	WithPackage(name string) CatalogFSBuilder
	WithChannel(pkg, name string, entries ...ChannelEntry) CatalogFSBuilder
	WithBundle(pkg, version string, opts ...BundleOption) CatalogFSBuilder
	WithDeprecation(pkg string, entries ...declcfg.DeprecationEntry) CatalogFSBuilder
	WithCustom(pkg, schema, name string, fieldKVs ...any) CatalogFSBuilder
	Build() fstest.MapFS
}

// Builder returns a new CatalogFSBuilder.
func Builder() CatalogFSBuilder {
	return &builder{}
}

type builder struct {
	packages     []packageBlob
	channels     []channelBlob
	bundles      []bundleBlob
	deprecations []declcfg.Deprecation
	customs      []customBlob
}

type customBlob struct {
	pkg      string
	schema   string
	name     string
	fieldKVs []any
}

type packageBlob struct {
	name string
}

type channelBlob struct {
	pkg     string
	name    string
	entries []ChannelEntry
}

type bundleBlob struct {
	pkg      string
	version  string
	name     string
	image    string
	imageSet bool
	release  string
}

func (b *builder) WithPackage(name string) CatalogFSBuilder {
	b.packages = append(b.packages, packageBlob{name: name})
	return b
}

func (b *builder) WithChannel(pkg, name string, entries ...ChannelEntry) CatalogFSBuilder {
	b.channels = append(b.channels, channelBlob{pkg: pkg, name: name, entries: entries})
	return b
}

func (b *builder) WithDeprecation(pkg string, entries ...declcfg.DeprecationEntry) CatalogFSBuilder {
	b.deprecations = append(b.deprecations, declcfg.Deprecation{
		Schema:  declcfg.SchemaDeprecation,
		Package: pkg,
		Entries: entries,
	})
	return b
}

// WithCustom adds a custom blob to the catalog filesystem.
// The blob includes schema, package, and name fields, plus any additional
// fields from fieldKVs (alternating key, value pairs). Panics if len(fieldKVs)
// is odd or any key is not a string.
func (b *builder) WithCustom(pkg, schema, name string, fieldKVs ...any) CatalogFSBuilder {
	if len(fieldKVs)%2 != 0 {
		panic("catalogfs: WithCustom fieldKVs must be key-value pairs")
	}
	for i := 0; i < len(fieldKVs); i += 2 {
		if _, ok := fieldKVs[i].(string); !ok {
			panic(fmt.Sprintf("catalogfs: WithCustom key at index %d is %T, want string", i, fieldKVs[i]))
		}
	}
	b.customs = append(b.customs, customBlob{pkg: pkg, schema: schema, name: name, fieldKVs: fieldKVs})
	return b
}

func (b *builder) WithBundle(pkg, version string, opts ...BundleOption) CatalogFSBuilder {
	bndl := bundleBlob{pkg: pkg, version: version}
	for _, opt := range opts {
		opt(&bndl)
	}
	b.bundles = append(b.bundles, bndl)
	return b
}

func (b *builder) Build() fstest.MapFS {
	fs := fstest.MapFS{}
	addBlob := func(path string, data []byte) {
		if existing, ok := fs[path]; ok {
			existing.Data = append(existing.Data, '\n')
			existing.Data = append(existing.Data, data...)
		} else {
			fs[path] = &fstest.MapFile{Data: data}
		}
	}
	for _, p := range b.packages {
		path := fmt.Sprintf("%s/olm.package.json", p.name)
		addBlob(path, mustMarshal(fbcPackageJSON{Schema: "olm.package", Name: p.name}))
	}
	for _, ch := range b.channels {
		entries := make([]fbcChannelEntryJSON, len(ch.entries))
		for i, e := range ch.entries {
			entries[i] = resolveEntry(ch.pkg, e)
		}
		path := fmt.Sprintf("%s/olm.channel.%s.json", ch.pkg, ch.name)
		addBlob(path, mustMarshal(fbcChannelJSON{
			Schema:  "olm.channel",
			Package: ch.pkg,
			Name:    ch.name,
			Entries: entries,
		}))
	}
	for _, bndl := range b.bundles {
		name := bndl.name
		if name == "" {
			name = fmt.Sprintf("%s.v%s", bndl.pkg, bndl.version)
		}
		image := bndl.image
		if !bndl.imageSet && image == "" {
			image = fmt.Sprintf("quay.io/%s/bundle:v%s", bndl.pkg, bndl.version)
		}
		prop := fbcPackagePropertyValue{
			PackageName: bndl.pkg,
			Version:     bndl.version,
		}
		if bndl.release != "" {
			prop.Release = bndl.release
		}
		path := fmt.Sprintf("%s/olm.bundle.%s.json", bndl.pkg, name)
		addBlob(path, mustMarshal(fbcBundleJSON{
			Schema:  "olm.bundle",
			Package: bndl.pkg,
			Name:    name,
			Image:   image,
			Properties: []fbcPropertyJSON{{
				Type:  "olm.package",
				Value: prop,
			}},
		}))
	}
	for _, d := range b.deprecations {
		path := fmt.Sprintf("%s/olm.deprecations.json", d.Package)
		addBlob(path, mustMarshal(d))
	}
	for _, c := range b.customs {
		blob := map[string]any{
			"schema": c.schema,
			"name":   c.name,
		}
		if c.pkg != "" {
			blob["package"] = c.pkg
		}
		for i := 0; i < len(c.fieldKVs); i += 2 {
			blob[c.fieldKVs[i].(string)] = c.fieldKVs[i+1]
		}
		path := fmt.Sprintf("%s.%s.json", c.schema, c.name)
		if c.pkg != "" {
			path = fmt.Sprintf("%s/%s", c.pkg, path)
		}
		addBlob(path, mustMarshal(blob))
	}
	return fs
}

// ChannelEntry represents an entry in a channel.
type ChannelEntry struct {
	version   string
	replaces  string
	skips     []string
	skipRange string
}

// Entry creates a channel entry for the bundle at the given version.
// During Build, the entry's bundle name is derived as "<package>.v<version>"
// using the package from the containing WithChannel call.
func Entry(version string, opts ...EntryOption) ChannelEntry {
	e := ChannelEntry{version: version}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}

// EntryOption configures a ChannelEntry.
type EntryOption func(*ChannelEntry)

// Replaces sets the replaces field. The version is resolved to
// "<package>.v<version>" during Build.
func Replaces(version string) EntryOption {
	return func(e *ChannelEntry) { e.replaces = version }
}

// Skips sets the skips list. Each version is resolved to
// "<package>.v<version>" during Build.
func Skips(versions ...string) EntryOption {
	return func(e *ChannelEntry) { e.skips = versions }
}

// SkipRange sets the skipRange field (raw semver range string, not resolved).
func SkipRange(r string) EntryOption {
	return func(e *ChannelEntry) { e.skipRange = r }
}

// BundleOption configures a bundle.
type BundleOption func(*bundleBlob)

// WithName overrides the auto-derived bundle name (default: "<package>.v<version>").
func WithName(name string) BundleOption {
	return func(b *bundleBlob) { b.name = name }
}

// WithImage overrides the auto-derived image (default: "quay.io/<package>/bundle:v<version>").
func WithImage(image string) BundleOption {
	return func(b *bundleBlob) { b.image = image; b.imageSet = true }
}

// WithRelease sets the release field in the olm.package property.
func WithRelease(release string) BundleOption {
	return func(b *bundleBlob) { b.release = release }
}

func resolveEntry(pkg string, e ChannelEntry) fbcChannelEntryJSON {
	entry := fbcChannelEntryJSON{
		Name: fmt.Sprintf("%s.v%s", pkg, e.version),
	}
	if e.replaces != "" {
		entry.Replaces = fmt.Sprintf("%s.v%s", pkg, e.replaces)
	}
	if len(e.skips) > 0 {
		entry.Skips = make([]string, len(e.skips))
		for i, s := range e.skips {
			entry.Skips[i] = fmt.Sprintf("%s.v%s", pkg, s)
		}
	}
	if e.skipRange != "" {
		entry.SkipRange = e.skipRange
	}
	return entry
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("catalogfs: marshal error: %w", err))
	}
	return data
}

// JSON serialization types

type fbcPackageJSON struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

type fbcChannelJSON struct {
	Schema  string                `json:"schema"`
	Package string                `json:"package"`
	Name    string                `json:"name"`
	Entries []fbcChannelEntryJSON `json:"entries"`
}

type fbcChannelEntryJSON struct {
	Name      string   `json:"name"`
	Replaces  string   `json:"replaces,omitempty"`
	Skips     []string `json:"skips,omitempty"`
	SkipRange string   `json:"skipRange,omitempty"`
}

type fbcBundleJSON struct {
	Schema     string            `json:"schema"`
	Package    string            `json:"package"`
	Name       string            `json:"name"`
	Image      string            `json:"image,omitempty"`
	Properties []fbcPropertyJSON `json:"properties"`
}

type fbcPropertyJSON struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type fbcPackagePropertyValue struct {
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
	Release     string `json:"release,omitempty"`
}

// PackageDeprecation creates a package-level deprecation entry.
func PackageDeprecation(message string) declcfg.DeprecationEntry {
	return declcfg.DeprecationEntry{
		Reference: declcfg.PackageScopedReference{Schema: declcfg.SchemaPackage},
		Message:   message,
	}
}

// ChannelDeprecation creates a channel-level deprecation entry.
func ChannelDeprecation(name, message string) declcfg.DeprecationEntry {
	return declcfg.DeprecationEntry{
		Reference: declcfg.PackageScopedReference{Schema: declcfg.SchemaChannel, Name: name},
		Message:   message,
	}
}

// BundleDeprecation creates a bundle-level deprecation entry.
func BundleDeprecation(name, message string) declcfg.DeprecationEntry {
	return declcfg.DeprecationEntry{
		Reference: declcfg.PackageScopedReference{Schema: declcfg.SchemaBundle, Name: name},
		Message:   message,
	}
}
