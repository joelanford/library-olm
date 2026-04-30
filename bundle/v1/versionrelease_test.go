package bundlev1

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
)

func TestVersionReleaseCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b VersionRelease
		want int
	}{
		{
			name: "equal versions and releases",
			a:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("")},
			b:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("")},
			want: 0,
		},
		{
			name: "version takes precedence",
			a:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("rc2")},
			b:    VersionRelease{Version: semver.MustParse("2.0.0"), Release: MustParseRelease("rc1")},
			want: -1,
		},
		{
			name: "release breaks version tie",
			a:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("")},
			b:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("rc1")},
			want: -1,
		},
		{
			name: "release comparison",
			a:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("rc1")},
			b:    VersionRelease{Version: semver.MustParse("1.0.0"), Release: MustParseRelease("rc2")},
			want: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.Compare(tt.b))
		})
	}
}
