package bundlev1

import (
	"strings"

	bsemver "github.com/blang/semver/v4"
)

// Bundle represents a versioned unit of content in a catalog.
// Different catalog formats (registry+v1, Helm, registry+v2) provide
// their own implementations.
type Bundle interface {
	Name() string
	VersionRelease() VersionRelease
}

// NameVersionRelease is a bundle identity: name + version + release.
// It is the simplest Bundle implementation — just identity fields,
// no format-specific data.
type NameVersionRelease struct {
	BundleName string
	Version    bsemver.Version
	Release    Release
}

func (nvr NameVersionRelease) Name() string {
	return nvr.BundleName
}

func (nvr NameVersionRelease) VersionRelease() VersionRelease {
	return VersionRelease{Version: nvr.Version, Release: nvr.Release}
}

// Compare returns -1, 0, or 1 comparing nvr to other.
// Name is compared first; version and release break ties.
func (nvr NameVersionRelease) Compare(other NameVersionRelease) int {
	if c := strings.Compare(nvr.BundleName, other.BundleName); c != 0 {
		return c
	}
	return nvr.VersionRelease().Compare(other.VersionRelease())
}
