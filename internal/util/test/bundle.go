package test

import (
	"testing"

	bsemver "github.com/blang/semver/v4"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
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
