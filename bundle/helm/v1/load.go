package v1

import (
	"fmt"
	"io/fs"
	"path"

	"helm.sh/helm/v4/pkg/chart/loader/archive"
	"helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
)

// Chart is Helm's parsed chart type.
type Chart = v2.Chart

// FromFS loads a chart from chartFS without creating an archive.
func FromFS(chartFS fs.FS) (*Chart, error) {
	var files []*archive.BufferedFile
	err := fs.WalkDir(chartFS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := fs.ReadFile(chartFS, name)
		if err != nil {
			return err
		}
		files = append(files, &archive.BufferedFile{Name: path.Clean(name), Data: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking chart filesystem: %w", err)
	}
	ch, err := loader.LoadFiles(files)
	if err != nil {
		return nil, fmt.Errorf("loading Helm chart: %w", err)
	}
	return ch, nil
}
