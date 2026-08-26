package validate

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRelease(t *testing.T) {
	now := time.Now()
	chart := &v2.Chart{}
	values := map[string]any{"key": "value"}
	newRelease := func() *releasev1.Release {
		return &releasev1.Release{
			Name:      "release",
			Namespace: "namespace",
			Chart:     chart,
			Config:    values,
			Manifest:  "manifest",
			Info: &releasev1.Info{
				FirstDeployed: now,
				LastDeployed:  now,
				Description:   "Dry run complete",
				Status:        releasecommon.StatusPendingInstall,
			},
			Version:     1,
			ApplyMethod: string(releasev1.ApplyMethodServerSideApply),
		}
	}
	tests := []struct {
		name      string
		mutate    func(*releasev1.Release)
		wantError string
	}{
		{name: "expected fields"},
		{
			name:      "unexpected name",
			mutate:    func(release *releasev1.Release) { release.Name = "unexpected" },
			wantError: "name",
		},
		{
			name:      "hooks",
			mutate:    func(release *releasev1.Release) { release.Hooks = []*releasev1.Hook{{}} },
			wantError: "hooks",
		},
		{
			name:      "deleted",
			mutate:    func(release *releasev1.Release) { release.Info.Deleted = now },
			wantError: "info.deleted",
		},
		{
			name: "resources",
			mutate: func(release *releasev1.Release) {
				release.Info.Resources = map[string][]runtime.Object{"example": nil}
			},
			wantError: "info.resources",
		},
		{
			name:      "missing info",
			mutate:    func(release *releasev1.Release) { release.Info = nil },
			wantError: "without info",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			release := newRelease()
			if test.mutate != nil {
				test.mutate(release)
			}
			err := Release(release, chart, values, "release", "namespace")
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestReleaseDifferenceReporter(t *testing.T) {
	tests := []struct {
		name       string
		expected   map[string]any
		actual     map[string]any
		wantFields []string
	}{
		{
			name:       "map field",
			expected:   map[string]any{"name": "expected"},
			actual:     map[string]any{"name": "actual"},
			wantFields: []string{"name"},
		},
		{
			name:       "slice index",
			expected:   map[string]any{"hooks": []any{map[string]any{"name": "expected"}}},
			actual:     map[string]any{"hooks": []any{map[string]any{"name": "actual"}}},
			wantFields: []string{"hooks[0].name"},
		},
		{
			name:       "fields are sorted",
			expected:   map[string]any{"second": "expected", "first": "expected"},
			actual:     map[string]any{"second": "actual", "first": "actual"},
			wantFields: []string{"first", "second"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reporter := &releaseDifferenceReporter{}
			require.False(t, cmp.Equal(test.expected, test.actual, cmp.Reporter(reporter)))
			require.Equal(t, test.wantFields, reporter.fields())
		})
	}
}
