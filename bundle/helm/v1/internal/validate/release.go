package validate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
	"helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

// Release rejects release fields that cannot be represented in plain manifests.
func Release(actual *releasev1.Release, chart *v2.Chart, values map[string]any, releaseName, namespace string) error {
	if actual.Info == nil {
		return fmt.Errorf("helm templating returned a release without info")
	}
	expected := &releasev1.Release{
		Name:      releaseName,
		Namespace: namespace,
		Chart:     chart,
		Config:    values,
		Manifest:  actual.Manifest,
		Info: &releasev1.Info{
			FirstDeployed: actual.Info.FirstDeployed,
			LastDeployed:  actual.Info.LastDeployed,
			Description:   "Dry run complete",
			Status:        releasecommon.StatusPendingInstall,
			Notes:         actual.Info.Notes,
		},
		Version:     1,
		ApplyMethod: string(releasev1.ApplyMethodServerSideApply),
	}
	expectedFields, err := fields(expected)
	if err != nil {
		return err
	}
	actualFields, err := fields(actual)
	if err != nil {
		return err
	}
	reporter := &releaseDifferenceReporter{}
	if !cmp.Equal(expectedFields, actualFields, cmp.Reporter(reporter)) {
		return fmt.Errorf("helm templating returned unexpected release fields: %s", strings.Join(reporter.fields(), ", "))
	}
	return nil
}

func fields(release *releasev1.Release) (map[string]any, error) {
	data, err := json.Marshal(release)
	if err != nil {
		return nil, fmt.Errorf("serializing helm release: %w", err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("decoding helm release: %w", err)
	}
	return fields, nil
}

type releaseDifferenceReporter struct {
	path            cmp.Path
	differingFields map[string]struct{}
}

func (r *releaseDifferenceReporter) PushStep(step cmp.PathStep) {
	r.path = append(r.path, step)
}

func (r *releaseDifferenceReporter) Report(result cmp.Result) {
	if result.Equal() || len(r.path) < 2 {
		return
	}
	if r.differingFields == nil {
		r.differingFields = map[string]struct{}{}
	}
	var path string
	for _, step := range r.path {
		switch step := step.(type) {
		case cmp.MapIndex:
			path = appendPathField(path, fmt.Sprint(step.Key().Interface()))
		case cmp.SliceIndex:
			path += step.String()
		}
	}
	r.differingFields[path] = struct{}{}
}

func appendPathField(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

func (r *releaseDifferenceReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *releaseDifferenceReporter) fields() []string {
	fields := make([]string, 0, len(r.differingFields))
	for field := range r.differingFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}
