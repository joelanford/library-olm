package bundlev1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRelease(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantStr string
		wantErr bool
	}{
		{name: "empty", input: "", wantStr: ""},
		{name: "single numeric", input: "1", wantStr: "1"},
		{name: "single alpha", input: "rc1", wantStr: "rc1"},
		{name: "dotted", input: "beta.2", wantStr: "beta.2"},
		{name: "multi-part", input: "1.pre.0", wantStr: "1.pre.0"},
		{name: "invalid chars", input: "rc@1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRelease(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStr, r.String())
		})
	}
}

func TestReleaseCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{name: "both empty", a: "", b: "", want: 0},
		{name: "empty < non-empty", a: "", b: "rc1", want: -1},
		{name: "non-empty > empty", a: "rc1", b: "", want: 1},
		{name: "equal single", a: "rc1", b: "rc1", want: 0},
		{name: "numeric by value", a: "2", b: "10", want: -1},
		{name: "alpha lexical", a: "beta", b: "rc", want: -1},
		{name: "numeric before alpha", a: "1", b: "beta", want: -1},
		{name: "alpha after numeric", a: "beta", b: "1", want: 1},
		{name: "fewer before more", a: "rc", b: "rc.1", want: -1},
		{name: "more after fewer", a: "rc.1", b: "rc", want: 1},
		{name: "dotted comparison", a: "beta.1", b: "beta.2", want: -1},
		{name: "equal dotted", a: "rc.1", b: "rc.1", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := MustParseRelease(tt.a)
			b := MustParseRelease(tt.b)
			assert.Equal(t, tt.want, a.Compare(b))
		})
	}
}

func TestReleaseIsEmpty(t *testing.T) {
	assert.True(t, Release{}.IsEmpty())
	assert.True(t, MustParseRelease("").IsEmpty())
	assert.False(t, MustParseRelease("rc1").IsEmpty())
}
