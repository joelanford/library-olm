package test

import (
	"context"
	"encoding/json"
	"testing"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
)

// BundleIdentity is a simple bundlev1.BundleIdentity for use in tests.
type BundleIdentity struct {
	BundleID bundlev1.BundleID
	NVR      bundlev1.NameVersionRelease
}

func (b BundleIdentity) ID() bundlev1.BundleID                           { return b.BundleID }
func (b BundleIdentity) NameVersionRelease() bundlev1.NameVersionRelease { return b.NVR }

// NewBundleIdentity creates a BundleIdentity with a derived ID of "{name}.v{version}".
// Tests that need a custom ID can use BundleIdentity directly.
func NewBundleIdentity(t *testing.T, name, version, release string) bundlev1.BundleIdentity {
	t.Helper()
	v, err := bsemver.Parse(version)
	if err != nil {
		t.Fatalf("parse version %q: %v", version, err)
	}
	r, err := bundlev1.ParseRelease(release)
	if err != nil {
		t.Fatalf("parse release %q: %v", release, err)
	}
	id := name + ".v" + version
	if release != "" {
		id += "-" + release
	}
	return BundleIdentity{
		BundleID: bundlev1.BundleID(id),
		NVR:      bundlev1.NameVersionRelease{Name: name, Version: v, Release: r},
	}
}

// Bundle is a configurable bundlev1.Bundle for tests.
type Bundle struct {
	bundlev1.BundleIdentity
	BundleURI      string
	BundleMetadata bundlev1.BundleMetadata
	MetadataErr    error
	MetadataCalls  int
	Properties     map[string]json.RawMessage
	PropertyErr    error
}

// NewBundle creates a Bundle with a derived ID of "{name}.v{version}".
func NewBundle(t *testing.T, name, version, release string) *Bundle {
	t.Helper()
	return &Bundle{BundleIdentity: NewBundleIdentity(t, name, version, release)}
}

func (b *Bundle) URI() string { return b.BundleURI }

func (b *Bundle) Metadata(context.Context) (bundlev1.BundleMetadata, error) {
	b.MetadataCalls++
	return b.BundleMetadata, b.MetadataErr
}

func (b *Bundle) Property(_ context.Context, key string) (json.RawMessage, error) {
	return b.Properties[key], b.PropertyErr
}

// DeprecatedBundle adds catalogv1.Deprecated to a Bundle.
type DeprecatedBundle struct {
	*Bundle
	Message string
}

func (b *DeprecatedBundle) DeprecationMessage() string { return b.Message }

var _ bundlev1.Bundle = (*Bundle)(nil)
var _ bundlev1.Bundle = (*DeprecatedBundle)(nil)
var _ catalogv1.Deprecated = (*DeprecatedBundle)(nil)
