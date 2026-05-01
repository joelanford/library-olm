package bundlev1

import (
	"testing"

	bsemver "github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
)

func TestNameVersionReleaseCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b NameVersionRelease
		want int
	}{
		{
			name: "equal",
			a:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("1.0.0")},
			b:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("1.0.0")},
			want: 0,
		},
		{
			name: "name takes precedence",
			a:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("2.0.0")},
			b:    NameVersionRelease{Name: "b", Version: bsemver.MustParse("1.0.0")},
			want: -1,
		},
		{
			name: "version breaks name tie",
			a:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("1.0.0")},
			b:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("2.0.0")},
			want: -1,
		},
		{
			name: "release breaks version tie",
			a:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("1.0.0"), Release: MustParseRelease("")},
			b:    NameVersionRelease{Name: "a", Version: bsemver.MustParse("1.0.0"), Release: MustParseRelease("rc1")},
			want: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.a.Compare(tt.b))
		})
	}
}
