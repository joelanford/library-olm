package v1

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func baseChart(t *testing.T, mutate ...func(*Chart)) *Chart {
	t.Helper()
	chart, err := FromFS(os.DirFS("testdata/base-chart"))
	require.NoError(t, err)
	for _, mutate := range mutate {
		mutate(chart)
	}
	return chart
}

type toPlainManifestsTestCase struct {
	name        string
	chart       *Chart
	releaseName string
	namespace   string
	options     []RenderOption
	assert      func(*testing.T, []client.Object, error)
}

func TestToPlainManifests(t *testing.T) {
	tests := []toPlainManifestsTestCase{
		{
			name:        "renders base chart",
			chart:       baseChart(t),
			releaseName: "release",
			namespace:   "namespace",
			options: []RenderOption{
				WithValues(map[string]any{
					"parent":   map[string]any{"overridden": "user-parent-override", "requiredValue": "user-required-value", "replicas": 3, "servicePort": 9090},
					"child":    map[string]any{"userOverride": "user-override"},
					"optional": map[string]any{"enabled": false},
					"global":   map[string]any{"shared": "user-global-override"},
				}),
				WithKubeVersion(&version.Info{GitVersion: "v1.30.1"}),
				WithAPIVersions(
					[]*metav1.APIGroup{{Versions: []metav1.GroupVersionForDiscovery{{GroupVersion: "v1"}}}},
					[]*metav1.APIResourceList{{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Kind: "Deployment"}}}},
				),
			},
			assert: func(t *testing.T, objects []client.Object, err error) {
				require.NoError(t, err)
				for _, object := range objects {
					_, ok := object.(*unstructured.Unstructured)
					require.True(t, ok)
				}
				parent := object(t, objects, "ConfigMap", "release-parent")
				parentData := parent.Object["data"].(map[string]any)
				require.Equal(t, map[string]any{
					"defaultOnly": "parent-default", "overridden": "user-parent-override", "required": "user-required-value", "global": "user-global-override",
					"releaseName": "release", "releaseNamespace": "namespace", "releaseService": "Helm", "isInstall": "false", "isUpgrade": "true", "revision": "1",
					"chartName": "base-chart", "chartVersion": "1.0.0", "appVersion": "2.0.0", "file": "fixture message", "included": "base-label", "library": "library-label", "templated": "template-value",
					"kubeVersion": "v1.30.1", "kubeMajor": "1", "kubeMinor": "30", "hasV1": "true", "hasDeployment": "true",
				}, withoutTemplateMetadata(parentData))
				require.Contains(t, parentData["templateName"], "templates/configmap.yaml")
				require.Equal(t, "base-chart/templates", parentData["templateBasePath"])
				require.Equal(t, "base-chart-1.0.0", parent.GetLabels()["helm.sh/chart"])
				require.Equal(t, "base-chart", parent.GetLabels()["app.kubernetes.io/name"])

				child := object(t, objects, "ConfigMap", "release-child")
				require.Equal(t, map[string]any{"childDefaultOnly": "child-default", "parentOverride": "parent-override", "userOverride": "user-override", "global": "user-global-override"}, child.Object["data"])
				require.Nil(t, objectOrNil(objects, "ConfigMap", "release-optional"))
				require.NotNil(t, objectOrNil(objects, "CustomResourceDefinition", "examples.example.com"))
				service := object(t, objects, "Service", "release")
				deployment := object(t, objects, "Deployment", "release")
				require.IsType(t, json.Number(""), service.Object["spec"].(map[string]any)["ports"].([]any)[0].(map[string]any)["port"])
				require.IsType(t, json.Number(""), deployment.Object["spec"].(map[string]any)["replicas"])
				require.Less(t, indexOf(objects, parent), indexOf(objects, service))
				require.Less(t, indexOf(objects, service), indexOf(objects, deployment))
				require.Len(t, objects, 5)
			},
		},
		{name: "rejects invalid Kubernetes version", chart: baseChart(t), releaseName: "release", namespace: "namespace", options: []RenderOption{WithKubeVersion(&version.Info{GitVersion: "invalid"})}, assert: requireError("parsing Kubernetes version")},
		{name: "rejects nil chart", releaseName: "release", namespace: "namespace", assert: requireError("chart must not be nil")},
		{name: "rejects empty release name", chart: baseChart(t), namespace: "namespace", assert: requireError("release name must not be empty")},
		{name: "rejects empty namespace", chart: baseChart(t), releaseName: "release", assert: requireError("namespace must not be empty")},
		{name: "rejects parent schema violation", chart: baseChart(t), releaseName: "release", namespace: "namespace", options: []RenderOption{WithValues(map[string]any{"parent": map[string]any{"overridden": 1}})}, assert: requireError("overridden")},
		{name: "rejects child schema violation", chart: baseChart(t), releaseName: "release", namespace: "namespace", options: []RenderOption{WithValues(map[string]any{"child": map[string]any{"userOverride": 1}})}, assert: requireError("userOverride")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects, err := ToPlainManifests(test.chart, test.releaseName, test.namespace, test.options...)
			test.assert(t, objects, err)
		})
	}
}

func TestToPlainManifestsUnsupportedCharts(t *testing.T) {
	tests := []toPlainManifestsTestCase{
		{name: "hook output", chart: baseChart(t, appendTemplate("hook.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hook\n  annotations:\n    helm.sh/hook: pre-install\n")), releaseName: "release", namespace: "namespace", assert: requireUnsupportedChart("hooks")},
		{name: "disallowed Helm metadata", chart: baseChart(t, appendTemplate("metadata.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: metadata\n  annotations:\n    helm.sh/resource-policy: keep\n  labels:\n    helm.sh/disallowed: value\n")), releaseName: "release", namespace: "namespace", assert: requireUnsupportedChart("helm.sh/resource-policy", "helm.sh/disallowed")},
		{name: "collects independent validation failures", chart: baseChart(t, appendTemplate("hook.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hook\n  annotations:\n    helm.sh/hook: pre-install\n"), appendTemplate("metadata.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: metadata\n  annotations:\n    helm.sh/resource-policy: keep\n")), releaseName: "release", namespace: "namespace", assert: requireUnsupportedChart("hooks", "helm.sh/resource-policy")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects, err := ToPlainManifests(test.chart, test.releaseName, test.namespace, test.options...)
			test.assert(t, objects, err)
		})
	}
}

func TestToPlainManifestsDeniedFunctions(t *testing.T) {
	for _, test := range []struct{ name, expression string }{
		{"randAlphaNum", `randAlphaNum 8`}, {"now", `now`}, {"uuidv4", `uuidv4`}, {"genPrivateKey", `genPrivateKey "rsa"`}, {"getHostByName", `getHostByName "example.com"`}, {"lookup", `lookup "v1" "ConfigMap" "namespace" "example"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ToPlainManifests(baseChart(t, appendTemplate("denied.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ "+test.expression+" }}\n")), "release", "namespace")
			requireUnsupportedChart(test.name)(t, nil, err)
		})
	}
}

func TestToPlainManifestsCopiesChart(t *testing.T) {
	chart := baseChart(t)
	for _, test := range []struct {
		enabled bool
		want    string
	}{{false, "release-parent"}, {true, "release-child"}} {
		objects, err := ToPlainManifests(chart, "release", "namespace", WithValues(map[string]any{"child": map[string]any{"enabled": test.enabled}, "optional": map[string]any{"enabled": false}}))
		require.NoError(t, err)
		if test.enabled {
			require.NotNil(t, objectOrNil(objects, "ConfigMap", test.want))
		} else {
			require.Nil(t, objectOrNil(objects, "ConfigMap", "release-child"))
		}
	}
}

func TestParseManifest(t *testing.T) {
	for _, test := range []struct {
		name, manifest, wantError string
		wantNames                 []string
	}{
		{name: "multiple documents", manifest: strings.Join([]string{"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b"}, "\n---\n"), wantNames: []string{"a", "b"}},
		{name: "malformed document", manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: [invalid", wantError: "manifest-0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			objects, err := parseManifest(test.manifest)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantNames, []string{objects[0].GetName(), objects[1].GetName()})
		})
	}
}

func appendTemplate(name, contents string) func(*Chart) {
	return func(chart *Chart) {
		chart.Templates = append(chart.Templates, &common.File{Name: "templates/" + name, Data: []byte(contents)})
	}
}

func object(t *testing.T, objects []client.Object, kind, name string) *unstructured.Unstructured {
	t.Helper()
	result := objectOrNil(objects, kind, name)
	require.NotNil(t, result, "%s %s not found", kind, name)
	return result
}

func objectOrNil(objects []client.Object, kind, name string) *unstructured.Unstructured {
	for _, item := range objects {
		object := item.(*unstructured.Unstructured)
		if object.GetKind() == kind && object.GetName() == name {
			return object
		}
	}
	return nil
}

func indexOf(objects []client.Object, want client.Object) int {
	for i, object := range objects {
		if object == want {
			return i
		}
	}
	return -1
}
func withoutTemplateMetadata(data map[string]any) map[string]any {
	result := make(map[string]any, len(data)-2)
	for key, value := range data {
		if key != "templateName" && key != "templateBasePath" {
			result[key] = value
		}
	}
	return result
}
func requireError(want string) func(*testing.T, []client.Object, error) {
	return func(t *testing.T, _ []client.Object, err error) { require.ErrorContains(t, err, want) }
}
func requireUnsupportedChart(wantErrors ...string) func(*testing.T, []client.Object, error) {
	return func(t *testing.T, _ []client.Object, err error) {
		var unsupported *UnsupportedChart
		require.ErrorAs(t, err, &unsupported)
		for _, want := range wantErrors {
			require.ErrorContains(t, err, want)
		}
	}
}
