package v1

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestFromFS(t *testing.T) {
	tests := []struct {
		name   string
		fs     fs.FS
		assert func(*testing.T, *Chart, error)
	}{
		{
			name: "base chart",
			fs:   os.DirFS("testdata/base-chart"),
			assert: func(t *testing.T, chart *Chart, err error) {
				require.NoError(t, err)
				require.Equal(t, "base-chart", chart.Name())
				require.Equal(t, "parent-default", chart.Values["parent"].(map[string]any)["defaultOnly"])
				require.NotEmpty(t, chart.Schema)
				require.Len(t, chart.Templates, 5)
				require.Len(t, chart.CRDs(), 1)
				require.Len(t, chart.Dependencies(), 3)
				require.Contains(t, []string{chart.Dependencies()[0].Name(), chart.Dependencies()[1].Name(), chart.Dependencies()[2].Name()}, "child")
				require.Contains(t, []string{chart.Dependencies()[0].Name(), chart.Dependencies()[1].Name(), chart.Dependencies()[2].Name()}, "library")
			},
		},
		{
			name: "missing Chart.yaml",
			fs:   fstest.MapFS{"values.yaml": {Data: []byte("name: value\n")}},
			assert: func(t *testing.T, _ *Chart, err error) {
				require.ErrorContains(t, err, "Chart.yaml")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chart, err := FromFS(test.fs)
			test.assert(t, chart, err)
		})
	}
}
