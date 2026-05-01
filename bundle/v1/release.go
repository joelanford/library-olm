package bundlev1

import (
	"fmt"
	"strings"

	bsemver "github.com/blang/semver/v4"
)

// Release is a dot-separated sequence of identifiers that qualifies
// a bundle version (e.g. "rc1", "beta.2", "1.pre.0").
//
// Sorting rules:
//   - An empty Release sorts lower than any non-empty Release.
//   - Identifiers are compared left-to-right.
//   - A purely numeric identifier compares by value (2 < 10).
//   - An alphanumeric identifier compares lexically ("beta" < "rc").
//   - A numeric identifier sorts before an alphanumeric identifier.
//   - When all preceding identifiers are equal, fewer identifiers
//     sort before more (e.g. "rc" < "rc.1").
type Release struct {
	ids []bsemver.PRVersion
}

// ParseRelease parses a dot-separated release string into a Release.
// An empty string returns a zero-value Release (no identifiers).
func ParseRelease(s string) (Release, error) {
	if s == "" {
		return Release{}, nil
	}
	parts := strings.Split(s, ".")
	ids := make([]bsemver.PRVersion, len(parts))
	for i, p := range parts {
		pr, err := bsemver.NewPRVersion(p)
		if err != nil {
			return Release{}, fmt.Errorf("invalid release identifier %q: %w", p, err)
		}
		ids[i] = pr
	}
	return Release{ids: ids}, nil
}

// MustParseRelease is like ParseRelease but panics on error.
func MustParseRelease(s string) Release {
	r, err := ParseRelease(s)
	if err != nil {
		panic(err)
	}
	return r
}

// IsEmpty returns true if the Release has no identifiers.
func (r Release) IsEmpty() bool {
	return len(r.ids) == 0
}

// Compare returns -1, 0, or 1 comparing r to other.
func (r Release) Compare(other Release) int {
	if r.IsEmpty() && other.IsEmpty() {
		return 0
	}
	if r.IsEmpty() {
		return -1
	}
	if other.IsEmpty() {
		return 1
	}

	for i := range min(len(r.ids), len(other.ids)) {
		if c := r.ids[i].Compare(other.ids[i]); c != 0 {
			return c
		}
	}

	switch {
	case len(r.ids) < len(other.ids):
		return -1
	case len(r.ids) > len(other.ids):
		return 1
	default:
		return 0
	}
}

// String returns the dot-separated release string, or "" if empty.
func (r Release) String() string {
	if len(r.ids) == 0 {
		return ""
	}
	parts := make([]string, len(r.ids))
	for i, id := range r.ids {
		parts[i] = id.String()
	}
	return strings.Join(parts, ".")
}
