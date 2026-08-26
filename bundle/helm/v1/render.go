package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"

	"github.com/mitchellh/copystructure"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/library-olm/bundle/helm/v1/internal/hermetic"
	chartvalidate "github.com/joelanford/library-olm/bundle/helm/v1/internal/validate"
)

type renderOptions struct {
	values    map[string]any
	kube      *version.Info
	groups    []*metav1.APIGroup
	resources []*metav1.APIResourceList
}
type RenderOption func(*renderOptions)

// UnsupportedChart reports that a chart uses a feature unsupported by this renderer.
type UnsupportedChart struct {
	Err error
}

func (e *UnsupportedChart) Error() string {
	return fmt.Sprintf("unsupported chart: %v", e.Err)
}

func (e *UnsupportedChart) Unwrap() error {
	return e.Err
}

func WithValues(values map[string]any) RenderOption {
	return func(options *renderOptions) { options.values = values }
}

// WithKubeVersion sets Capabilities.KubeVersion from a discovery ServerVersion response.
func WithKubeVersion(kubeVersion *version.Info) RenderOption {
	return func(options *renderOptions) { options.kube = kubeVersion }
}

// WithAPIVersions sets Capabilities.APIVersions from discovery group and resource responses.
func WithAPIVersions(groups []*metav1.APIGroup, resources []*metav1.APIResourceList) RenderOption {
	return func(options *renderOptions) {
		options.groups = groups
		options.resources = resources
	}
}

// ToPlainManifests renders a chart as unstructured client objects.
func ToPlainManifests(chrt *Chart, releaseName, namespace string, opts ...RenderOption) ([]client.Object, error) {
	if chrt == nil {
		return nil, fmt.Errorf("chart must not be nil")
	}
	if releaseName == "" {
		return nil, fmt.Errorf("release name must not be empty")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}
	var o renderOptions
	for _, opt := range opts {
		opt(&o)
	}
	caps, err := capabilities(o)
	if err != nil {
		return nil, err
	}
	config := buildConfig(caps)
	install := buildInstallAction(config, releaseName, namespace)
	chart, err := copyChart(chrt)
	if err != nil {
		return nil, err
	}
	release, err := render(install, chart, o.values)
	if err != nil {
		var unsupportedFunc *hermetic.UnsupportedTemplateFunction
		if errors.As(err, &unsupportedFunc) {
			return nil, &UnsupportedChart{Err: err}
		}
		return nil, err
	}
	return parseAndValidate(release, chart, o.values, releaseName, namespace)
}

func buildConfig(caps *common.Capabilities) *action.Configuration {
	config := action.NewConfiguration()
	config.Capabilities = caps
	config.KubeClient = &kubefake.PrintingKubeClient{Out: io.Discard}
	config.CustomTemplateFuncs = hermetic.Overrides()
	return config
}

func buildInstallAction(config *action.Configuration, releaseName, namespace string) *action.Install {
	install := action.NewInstall(config)
	install.DryRunStrategy = action.DryRunServer
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.Replace = true
	install.IsUpgrade = true
	install.IncludeCRDs = true
	return install
}

func render(install *action.Install, chart *Chart, values map[string]any) (*releasev1.Release, error) {
	rel, err := install.Run(chart, values)
	if err != nil {
		return nil, fmt.Errorf("rendering chart: %w", err)
	}
	release, ok := rel.(*releasev1.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected helm release type %T", rel)
	}
	return release, nil
}

func parseAndValidate(release *releasev1.Release, chart *Chart, values map[string]any, releaseName, namespace string) ([]client.Object, error) {
	objects, err := parseManifest(release.Manifest)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(
		chartvalidate.Release(release, chart, values, releaseName, namespace),
		chartvalidate.HelmMetadata(objects),
	); err != nil {
		return nil, &UnsupportedChart{Err: err}
	}
	return objects, nil
}

func copyChart(chrt *Chart) (*Chart, error) {
	chartCopy, err := copystructure.Copy(chrt)
	if err != nil {
		return nil, fmt.Errorf("copying chart for helm templating: %w", err)
	}
	copy, ok := chartCopy.(*Chart)
	if !ok {
		return nil, fmt.Errorf("unexpected copied chart type %T", chartCopy)
	}
	dependencies := make([]*Chart, 0, len(chrt.Dependencies()))
	for _, dependency := range chrt.Dependencies() {
		dependencyCopy, err := copyChart(dependency)
		if err != nil {
			return nil, err
		}
		dependencies = append(dependencies, dependencyCopy)
	}
	copy.SetDependencies(dependencies...)
	return copy, nil
}

func capabilities(options renderOptions) (*common.Capabilities, error) {
	caps := &common.Capabilities{}
	if options.kube != nil {
		kv, err := common.ParseKubeVersion(options.kube.GitVersion)
		if err != nil {
			return nil, fmt.Errorf("parsing Kubernetes version: %w", err)
		}
		caps.KubeVersion = *kv
	}
	for _, group := range options.groups {
		for _, groupVersion := range group.Versions {
			caps.APIVersions = append(caps.APIVersions, groupVersion.GroupVersion)
		}
	}
	for _, resources := range options.resources {
		for _, resource := range resources.APIResources {
			caps.APIVersions = append(caps.APIVersions, path.Join(resources.GroupVersion, resource.Kind))
		}
	}
	return caps, nil
}

func parseManifest(manifest string) ([]client.Object, error) {
	manifests := releaseutil.SplitManifests(manifest)
	manifestPaths := make(releaseutil.BySplitManifestsOrder, 0, len(manifests))
	for manifestPath := range manifests {
		manifestPaths = append(manifestPaths, manifestPath)
	}
	sort.Sort(manifestPaths)
	objects := make([]client.Object, 0, len(manifestPaths))
	for _, manifestPath := range manifestPaths {
		var raw map[string]any
		if err := yaml.Unmarshal([]byte(manifests[manifestPath]), &raw, func(dec *json.Decoder) *json.Decoder { dec.UseNumber(); return dec }); err != nil {
			return nil, fmt.Errorf("decoding manifest %q: %w", manifestPath, err)
		}
		objects = append(objects, &unstructured.Unstructured{Object: raw})
	}
	return objects, nil
}
