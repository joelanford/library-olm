package bundlev1

import (
	bsemver "github.com/blang/semver/v4"
)

// VersionRelease pairs a semver version with a release qualifier.
type VersionRelease struct {
	Version bsemver.Version
	Release Release
}

// Compare returns -1, 0, or 1 comparing vr to other.
// Version is compared first; Release breaks ties.
func (vr VersionRelease) Compare(other VersionRelease) int {
	if c := vr.Version.Compare(other.Version); c != 0 {
		return c
	}
	return vr.Release.Compare(other.Release)
}
