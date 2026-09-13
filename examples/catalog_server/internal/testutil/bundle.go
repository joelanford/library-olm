package testutil

import (
	"context"
	"encoding/json"
	"testing"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
)

type BundleIdentity struct {
	BundleID bundlev1.BundleID
	NVR      bundlev1.NameVersionRelease
}

func (b BundleIdentity) ID() bundlev1.BundleID                           { return b.BundleID }
func (b BundleIdentity) NameVersionRelease() bundlev1.NameVersionRelease { return b.NVR }

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

type Bundle struct {
	bundlev1.BundleIdentity
	BundleURI      string
	BundleMetadata model.BundleMetadata
	MetadataErr    error
	MetadataCalls  int
	Properties     map[string]json.RawMessage
	PropertyErr    error
}

func NewBundle(t *testing.T, name, version, release string) *Bundle {
	t.Helper()
	return &Bundle{BundleIdentity: NewBundleIdentity(t, name, version, release)}
}

func (b *Bundle) URI() string { return b.BundleURI }

func (b *Bundle) Property(_ context.Context, key string) (json.RawMessage, error) {
	if key == model.BundleMetadataProperty {
		b.MetadataCalls++
		if b.MetadataErr != nil {
			return nil, b.MetadataErr
		}
		return json.Marshal(b.BundleMetadata)
	}
	return b.Properties[key], b.PropertyErr
}

type DeprecatedBundle struct {
	*Bundle
	Message string
}

func (b *DeprecatedBundle) DeprecationMessage() string { return b.Message }

var _ bundlev1.Bundle = (*Bundle)(nil)
var _ bundlev1.Bundle = (*DeprecatedBundle)(nil)
