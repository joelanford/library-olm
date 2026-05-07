package bundlev1

import (
	"strings"

	bsemver "github.com/blang/semver/v4"
)

// BundleID is the unique identifier for a bundle within a catalog.
type BundleID string

// BundleIdentity identifies a bundle by its ID and name/version/release.
type BundleIdentity interface {
	ID() BundleID
	NameVersionRelease() NameVersionRelease
}

// Bundle represents a versioned unit of content in a catalog.
// Different catalog formats (registry+v1, Helm, registry+v2) provide
// their own implementations.
type Bundle interface {
	BundleIdentity
	URI() string
}

// NameVersionRelease is a bundle identity: package name + version + release.
type NameVersionRelease struct {
	// Name is the package name this bundle belongs to.
	Name    string
	Version bsemver.Version
	Release Release
}

func (nvr NameVersionRelease) VersionRelease() VersionRelease {
	return VersionRelease{Version: nvr.Version, Release: nvr.Release}
}

// Compare returns -1, 0, or 1 comparing nvr to other.
// Name is compared first; version and release break ties.
func (nvr NameVersionRelease) Compare(other NameVersionRelease) int {
	if c := strings.Compare(nvr.Name, other.Name); c != 0 {
		return c
	}
	return nvr.VersionRelease().Compare(other.VersionRelease())
}
