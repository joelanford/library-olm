package v1_test

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
)

var update = flag.Bool("update", false, "update golden files")

func TestRegression_RenderedOutputMatchesExpected(t *testing.T) {
	t.Cleanup(func() {
		if t.Failed() {
			t.Log("If the changes are intentional, update golden files with:\n" +
				"  go test -tags containers_image_openpgp -run TestRegression ./bundle/registry/v1/ -args -update")
		}
	})

	for _, tc := range []struct {
		name             string
		installNamespace string
		watchNamespace   string
		bundle           string
		deploymentConfig *registryv1.DeploymentConfig
	}{
		{
			name:             "all-namespaces",
			installNamespace: "argocd-system",
			bundle:           "argocd-operator.v0.6.0",
		},
		{
			name:             "single-namespace",
			installNamespace: "argocd-system",
			watchNamespace:   "argocd-watch",
			bundle:           "argocd-operator.v0.6.0",
		},
		{
			name:             "own-namespace",
			installNamespace: "argocd-system",
			watchNamespace:   "argocd-system",
			bundle:           "argocd-operator.v0.6.0",
		},
		{
			name:             "all-webhook-types",
			installNamespace: "webhook-system",
			bundle:           "webhook-operator.v0.0.5",
		},
		{
			name:             "with-deploymentconfig-options",
			installNamespace: "argocd-system",
			bundle:           "argocd-operator.v0.6.0",
			deploymentConfig: &registryv1.DeploymentConfig{
				NodeSelector: map[string]string{
					"kubernetes.io/os": "linux",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "some/key",
						Operator: corev1.TolerationOpEqual,
						Effect:   corev1.TaintEffectNoSchedule,
					},
					{
						Key:               "someother/key",
						Operator:          corev1.TolerationOpExists,
						Effect:            corev1.TaintEffectNoExecute,
						TolerationSeconds: ptr.To(int64(120)),
					},
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "CUSTOM_ENV_VAR",
						Value: "custom-value",
					},
					{
						Name:  "LOG_LEVEL",
						Value: "debug",
					},
				},
				EnvFrom: []corev1.EnvFromSource{
					{
						Prefix: "test",
					},
					{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "configmapForTest",
							},
						},
					},
					{
						SecretRef: &corev1.SecretEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "secretForTest",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "test-configmap-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "testVolumeConfigMap",
								},
							},
						},
					},
					{
						Name:         "test-emptydir-volume",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "test-configmap-volume",
						MountPath: "/test-volume-mount",
						ReadOnly:  true,
					},
				},
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "kubernetes.io/os",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"linux"},
										},
									},
									MatchFields: []corev1.NodeSelectorRequirement{
										{
											Key:      "key",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"val1", "val2"},
										},
									},
								},
							},
						},
					},
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app.kubernetes.io/name": "test-app",
									},
								},
								TopologyKey: "topKey",
								Namespaces:  []string{"test", "test2"},
							},
						},
					},
					PodAntiAffinity: &corev1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
							{
								Weight: 100,
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app.kubernetes.io/name": "test-app-2",
										},
									},
									TopologyKey: "topKey2",
								},
							},
						},
					},
				},
				Annotations: map[string]string{
					"foo":     "bar",
					"testkey": "testval",
				},
			},
		},
		{
			name:             "with-empty-affinity",
			installNamespace: "argocd-system",
			bundle:           "argocd-operator.v0.6.0",
			deploymentConfig: &registryv1.DeploymentConfig{
				Affinity: &corev1.Affinity{},
			},
		},
		{
			name:             "with-empty-affinity-subtype",
			installNamespace: "argocd-system",
			bundle:           "argocd-operator.v0.6.0",
			deploymentConfig: &registryv1.DeploymentConfig{
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundleFS := os.DirFS(filepath.Join("testdata", "regression", "bundles", tc.bundle))
			b, err := registryv1.FromFS(bundleFS)
			require.NoError(t, err)

			var opts []registryv1.RenderOption
			if tc.watchNamespace != "" {
				opts = append(opts, registryv1.WithTargetNamespaces(tc.watchNamespace))
			}
			if tc.deploymentConfig != nil {
				opts = append(opts, registryv1.WithDeploymentConfig(tc.deploymentConfig))
			}

			objs, err := registryv1.ToPlainManifests(b, tc.installNamespace, opts...)
			require.NoError(t, err)

			slices.SortFunc(objs, orderByKindNamespaceName)

			expectedDir := filepath.Join("testdata", "regression", "expected-manifests", tc.bundle, tc.name)

			if *update {
				updateGoldenFiles(t, expectedDir, objs)
				return
			}

			compareAgainstGoldenFiles(t, expectedDir, objs)
		})
	}
}

func orderByKindNamespaceName(a client.Object, b client.Object) int {
	return cmp.Or(
		cmp.Compare(a.GetObjectKind().GroupVersionKind().Kind, b.GetObjectKind().GroupVersionKind().Kind),
		cmp.Compare(a.GetNamespace(), b.GetNamespace()),
		cmp.Compare(a.GetName(), b.GetName()),
	)
}

func updateGoldenFiles(t *testing.T, dir string, objs []client.Object) {
	t.Helper()
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for idx, obj := range objs {
		kind := obj.GetObjectKind().GroupVersionKind().Kind
		fileName := fmt.Sprintf("%02d_%s_%s.yaml", idx, strings.ToLower(kind), obj.GetName())
		data, err := yaml.Marshal(obj)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), data, 0o600))
	}
	t.Logf("updated golden files in %s", dir)
}

func compareAgainstGoldenFiles(t *testing.T, expectedDir string, objs []client.Object) {
	t.Helper()

	renderedFiles := make(map[string][]byte)
	for idx, obj := range objs {
		kind := obj.GetObjectKind().GroupVersionKind().Kind
		fileName := fmt.Sprintf("%02d_%s_%s.yaml", idx, strings.ToLower(kind), obj.GetName())
		data, err := yaml.Marshal(obj)
		require.NoError(t, err)
		renderedFiles[fileName] = data
	}

	expectedFiles := make(map[string][]byte)
	entries, err := os.ReadDir(expectedDir)
	require.NoError(t, err, "failed to read expected directory %s", expectedDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(expectedDir, entry.Name()))
		require.NoError(t, err)
		expectedFiles[entry.Name()] = data
	}

	allNames := make(map[string]bool)
	for name := range expectedFiles {
		allNames[name] = true
	}
	for name := range renderedFiles {
		allNames[name] = true
	}

	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for name := range allNames {
			if !yield(name) {
				return
			}
		}
	}) {
		expected, hasExpected := expectedFiles[name]
		actual, hasActual := renderedFiles[name]

		switch {
		case !hasExpected:
			t.Errorf("unexpected extra file: %s", name)
		case !hasActual:
			t.Errorf("missing file: %s", name)
		case !bytes.Equal(expected, actual):
			diff := gocmp.Diff(string(expected), string(actual), cmpopts.AcyclicTransformer("lines", func(s string) []string {
				return strings.Split(s, "\n")
			}))
			t.Errorf("file content mismatch: %s\nDiff (-expected +actual):\n%s", name, diff)
		}
	}
}
