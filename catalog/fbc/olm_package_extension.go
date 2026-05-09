package fbc

import (
	"context"
	"encoding/json"
	"iter"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
)

// OLMPackageExtension hooks into the FBC import pipeline for the
// olm.package schema family. Per-blob callbacks run during the
// filesystem walk; FinalizePackage runs after normalization.
type OLMPackageExtension interface {
	OnPackage(declcfg.Package) (any, error)
	OnChannel(declcfg.Channel) (any, error)
	OnBundle(declcfg.Bundle) (any, error)
	OnDeprecation(declcfg.Deprecation) (any, error)
	OnOther(declcfg.Meta) (any, error)
	FinalizePackage(ctx context.Context, pkg PackageAccessor, w PropertyWriter) error
}

// PropertyWriter writes properties scoped to the current package.
// Path is relative to the package root (e.g., [] for the package graph,
// ["channelName"] for a channel graph). The implementation prepends
// the package name before delegating to the underlying Writer.
type PropertyWriter interface {
	SetBundleProperty(ctx context.Context, bundleName, key string, val any) error
	SetGraphProperty(ctx context.Context, path []string, key string, val any) error
}

// PackageAccessor provides read-only access to a package's staging data,
// including extension data from per-blob callbacks.
type PackageAccessor interface {
	Name() string
	ExtData() (json.RawMessage, error)
	Bundles() iter.Seq2[BundleAccessor, error]
	Channels() iter.Seq2[ChannelAccessor, error]
	Deprecations() iter.Seq2[DeprecationAccessor, error]
	Others() iter.Seq2[OtherAccessor, error]
}

// BundleAccessor provides read-only access to a staged bundle.
type BundleAccessor interface {
	Name() string
	Package() string
	Version() string
	Release() string
	Image() string
	ExtData() json.RawMessage
}

// ChannelAccessor provides read-only access to a staged channel.
type ChannelAccessor interface {
	Name() string
	Entries() iter.Seq2[ChannelEntryAccessor, error]
	ExtData() json.RawMessage
}

// ChannelEntryAccessor provides read-only access to a channel entry.
type ChannelEntryAccessor interface {
	BundleName() string
	Replaces() string
	Skips() []string
	SkipRange() string
}

// DeprecationAccessor provides read-only access to a staged deprecation.
type DeprecationAccessor interface {
	ExtData() json.RawMessage
}

// OtherAccessor provides read-only access to a staged "other" blob.
type OtherAccessor interface {
	Schema() string
	Name() string
	ExtData() json.RawMessage
}

// ImporterOption configures an Importer.
type ImporterOption func(*Importer)

// WithOLMPackageExtension registers an OLMPackageExtension with the importer.
func WithOLMPackageExtension(ext OLMPackageExtension) ImporterOption {
	return func(i *Importer) {
		i.olmPkgExt = ext
	}
}
