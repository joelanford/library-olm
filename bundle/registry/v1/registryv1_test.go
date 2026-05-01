package v1_test

import (
	"testing"
	"testing/fstest"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render/certproviders"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing/bundlefs"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing/clusterserviceversion"
)

func TestFromFS(t *testing.T) {
	t.Run("parses a valid bundle", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("my-operator").
			WithBundleProperty("test-key", "test-value").
			WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build())).
			Build()

		b, err := registryv1.FromFS(fs)
		require.NoError(t, err)
		assert.Equal(t, "my-operator", b.PackageName)
		assert.Equal(t, "my-operator.v1.0.0", b.CSV.Name)
	})

	t.Run("parses CRDs from bundle", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("my-operator").
			WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				Build())).
			WithBundleResource("crd.yaml", &apiextensionsv1.CustomResourceDefinition{
				TypeMeta:   metav1.TypeMeta{Kind: "CustomResourceDefinition", APIVersion: apiextensionsv1.SchemeGroupVersion.String()},
				ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "example.com",
					Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "foos", Singular: "foo", Kind: "Foo"},
					Scope: apiextensionsv1.NamespaceScoped,
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
						{Name: "v1", Served: true, Storage: true, Schema: &apiextensionsv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"},
						}},
					},
				},
			}).
			Build()

		b, err := registryv1.FromFS(fs)
		require.NoError(t, err)
		require.Len(t, b.CRDs, 1)
		assert.Equal(t, "foos.example.com", b.CRDs[0].Name)
	})

	t.Run("parses additional resources", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("my-operator").
			WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build())).
			WithBundleResource("configmap.yaml", &corev1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "my-config"},
			}).
			Build()

		b, err := registryv1.FromFS(fs)
		require.NoError(t, err)
		require.Len(t, b.Others, 1)
		assert.Equal(t, "my-config", b.Others[0].GetName())
	})

	t.Run("merges properties from metadata into CSV annotations", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("my-operator").
			WithBundleProperty("from-file", "value").
			WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithAnnotations(map[string]string{
					"olm.properties": `[{"type":"from-csv","value":"csv-val"}]`,
				}).
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build())).
			Build()

		b, err := registryv1.FromFS(fs)
		require.NoError(t, err)
		require.Contains(t, b.CSV.Annotations["olm.properties"], "from-csv")
		require.Contains(t, b.CSV.Annotations["olm.properties"], "from-file")
	})

	t.Run("fails when annotations.yaml is missing", func(t *testing.T) {
		fs := fstest.MapFS{}
		_, err := registryv1.FromFS(fs)
		require.Error(t, err)
	})

	t.Run("fails when CSV is missing", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("my-operator").
			Build()
		_, err := registryv1.FromFS(fs)
		require.Error(t, err)
	})
}

func TestToPlainManifests(t *testing.T) {
	t.Run("renders a simple AllNamespaces bundle", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-operator-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		require.NotEmpty(t, objs)

		// Should have: ServiceAccount, ClusterRole, ClusterRoleBinding, Deployment
		assertHasObjectOfKind(t, objs, "ServiceAccount")
		assertHasObjectOfKind(t, objs, "ClusterRole")
		assertHasObjectOfKind(t, objs, "ClusterRoleBinding")
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("renders OwnNamespace bundle with target namespace", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeOwnNamespace).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-operator-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace", registryv1.WithTargetNamespaces("my-namespace"))
		require.NoError(t, err)
		require.NotEmpty(t, objs)

		// OwnNamespace: should have Role/RoleBinding instead of ClusterRole/ClusterRoleBinding
		assertHasObjectOfKind(t, objs, "Role")
		assertHasObjectOfKind(t, objs, "RoleBinding")
	})

	t.Run("renders SingleNamespace bundle", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeSingleNamespace).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "install-ns", registryv1.WithTargetNamespaces("watch-ns"))
		require.NoError(t, err)
		require.NotEmpty(t, objs)
		assertHasObjectOfKind(t, objs, "Role")
		assertHasObjectOfKind(t, objs, "RoleBinding")
	})

	t.Run("renders CRDs from bundle", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "CustomResourceDefinition", APIVersion: apiextensionsv1.SchemeGroupVersion.String()},
					ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"},
				},
			},
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "CustomResourceDefinition")
	})

	t.Run("renders additional bundle resources", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
			Others: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "my-cm"},
				}},
			},
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertHasObjectOfKindAndName(t, objs, "ConfigMap", "my-cm")
	})

	t.Run("sets namespace on namespaced additional resources", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
			Others: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "my-cm"},
				}},
			},
		}

		objs, err := registryv1.ToPlainManifests(b, "target-ns")
		require.NoError(t, err)
		cm := findObjectByKindAndName(objs, "ConfigMap", "my-cm")
		require.NotNil(t, cm)
		assert.Equal(t, "target-ns", cm.GetNamespace())
	})

	t.Run("deployment has correct annotations", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)

		dep := findObjectByKind(objs, "Deployment")
		require.NotNil(t, dep)
		assert.Equal(t, "my-namespace", dep.GetNamespace())
	})

	t.Run("AllNamespaces promotes permissions to cluster permissions", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)

		// In AllNamespaces mode, permissions are promoted to ClusterRole/ClusterRoleBinding
		assertHasObjectOfKind(t, objs, "ClusterRole")
		assertHasObjectOfKind(t, objs, "ClusterRoleBinding")
		assertNoObjectOfKind(t, objs, "Role")
		assertNoObjectOfKind(t, objs, "RoleBinding")
	})

	t.Run("cluster permissions generate ClusterRole and ClusterRoleBinding", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithClusterPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "ClusterRole")
		assertHasObjectOfKind(t, objs, "ClusterRoleBinding")
	})
}

func TestToPlainManifests_Webhooks(t *testing.T) {
	t.Run("renders validating webhook", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "validate.foo.example.com",
					DeploymentName:          "my-operator-controller",
					ContainerPort:           443,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "ValidatingWebhookConfiguration")
		assertHasObjectOfKind(t, objs, "Service")
	})

	t.Run("renders mutating webhook", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.MutatingAdmissionWebhook,
					GenerateName:            "mutate.foo.example.com",
					DeploymentName:          "my-operator-controller",
					ContainerPort:           443,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "MutatingWebhookConfiguration")
		assertHasObjectOfKind(t, objs, "Service")
	})

	t.Run("renders conversion webhook with CRD", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ConversionWebhook,
					GenerateName:            "convert.foos.example.com",
					DeploymentName:          "my-operator-controller",
					ContainerPort:           443,
					ConversionCRDs:          []string{"foos.example.com"},
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "CustomResourceDefinition", APIVersion: apiextensionsv1.SchemeGroupVersion.String()},
					ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						PreserveUnknownFields: false,
					},
				},
			},
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)

		crd := findObjectByKind(objs, "CustomResourceDefinition")
		require.NotNil(t, crd)
	})

	t.Run("renders with OpenShift service CA cert provider", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "validate.foo.example.com",
					DeploymentName:          "my-operator-controller",
					ContainerPort:           443,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}

		provider := &certproviders.OpenshiftServiceCaCertificateProvider{}
		objs, err := registryv1.ToPlainManifests(b, "my-namespace", registryv1.WithCertificateProvider(provider))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "ValidatingWebhookConfiguration")
		assertHasObjectOfKind(t, objs, "Service")

		svc := findObjectByKind(objs, "Service")
		require.NotNil(t, svc)
		require.Contains(t, svc.GetAnnotations(), "service.beta.openshift.io/serving-cert-secret-name")
	})

	t.Run("renders with cert-manager cert provider", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "validate.foo.example.com",
					DeploymentName:          "my-operator-controller",
					ContainerPort:           443,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}

		provider := &certproviders.CertManagerCertificateProvider{}
		objs, err := registryv1.ToPlainManifests(b, "my-namespace", registryv1.WithCertificateProvider(provider))
		require.NoError(t, err)

		// cert-manager generates additional Issuer and Certificate objects
		assertHasObjectOfKind(t, objs, "Issuer")
		assertHasObjectOfKind(t, objs, "Certificate")

		vwc := findObjectByKind(objs, "ValidatingWebhookConfiguration")
		require.NotNil(t, vwc)
		require.Contains(t, vwc.GetAnnotations(), "cert-manager.io/inject-ca-from")
	})
}

func TestToPlainManifests_WithDeploymentConfig(t *testing.T) {
	baseBundle := func() registryv1.Bundle {
		return registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-operator-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "my-image:latest"}},
							},
						},
					},
				}).
				Build(),
		}
	}

	t.Run("applies env vars", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			Env: []corev1.EnvVar{
				{Name: "MY_ENV", Value: "my-value"},
			},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "my-namespace", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		require.NotEmpty(t, objs)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("applies tolerations", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			Tolerations: []corev1.Toleration{
				{Key: "key1", Operator: corev1.TolerationOpEqual, Value: "value1", Effect: corev1.TaintEffectNoSchedule},
			},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "my-namespace", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("applies node selector", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			NodeSelector: map[string]string{"disk": "ssd"},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "my-namespace", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("applies resource requirements", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: *ptr.To(corev1.ResourceList{corev1.ResourceCPU: {}}[corev1.ResourceCPU]),
				},
			},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "my-namespace", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("nil deployment config is a no-op", func(t *testing.T) {
		objs, err := registryv1.ToPlainManifests(baseBundle(), "my-namespace", registryv1.WithDeploymentConfig(nil))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})
}

func TestToPlainManifests_Validation(t *testing.T) {
	t.Run("rejects empty package name", func(t *testing.T) {
		b := registryv1.Bundle{
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.Error(t, err)
		require.Contains(t, err.Error(), "package name is empty")
	})

	t.Run("rejects duplicate deployment specs", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(
					v1alpha1.StrategyDeploymentSpec{Name: "dup"},
					v1alpha1.StrategyDeploymentSpec{Name: "dup"},
				).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate strategy deployment spec")
	})

	t.Run("rejects missing owned CRD", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.Error(t, err)
		require.Contains(t, err.Error(), "foos.example.com")
	})

	t.Run("rejects unsupported target namespaces", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeSingleNamespace).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace", registryv1.WithTargetNamespaces(""))
		require.Error(t, err)
		require.Contains(t, err.Error(), "do not support targeting all namespaces")
	})

	t.Run("rejects conversion webhook with non-AllNamespaces install mode", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeOwnNamespace, v1alpha1.InstallModeTypeAllNamespaces).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:           v1alpha1.ConversionWebhook,
					GenerateName:   "convert.foos",
					DeploymentName: "dep",
					ConversionCRDs: []string{"foos.example.com"},
				}).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"}},
			},
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace", registryv1.WithTargetNamespaces("my-namespace"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "conversion webhook")
	})

	t.Run("rejects webhook referencing non-existent deployment", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "validate.foo",
					DeploymentName:          "nonexistent-deployment",
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.Error(t, err)
		require.Contains(t, err.Error(), "non-existent deployment")
	})

	t.Run("rejects unsupported bundle resource kinds", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
			Others: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "UnsupportedKind",
					"metadata":   map[string]interface{}{"name": "bad-resource"},
				}},
			},
		}
		_, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.Error(t, err)
		require.Contains(t, err.Error(), "UnsupportedKind")
	})
}

func TestToPlainManifests_DeploymentConfigDetails(t *testing.T) {
	baseBundle := func() registryv1.Bundle {
		return registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
							},
						},
					},
				}).
				Build(),
		}
	}

	t.Run("applies envFrom", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			EnvFrom: []corev1.EnvFromSource{
				{ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
				}},
			},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "ns", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("applies volumes and volumeMounts", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			Volumes: []corev1.Volume{
				{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "data", MountPath: "/data"},
			},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "ns", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("applies affinity", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-east-1"}},
							}},
						},
					},
				},
				PodAffinity:     &corev1.PodAffinity{},
				PodAntiAffinity: &corev1.PodAntiAffinity{},
			},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "ns", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})

	t.Run("applies annotations", func(t *testing.T) {
		dc := &registryv1.DeploymentConfig{
			Annotations: map[string]string{"custom-key": "custom-value"},
		}
		objs, err := registryv1.ToPlainManifests(baseBundle(), "ns", registryv1.WithDeploymentConfig(dc))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Deployment")
	})
}

func TestToPlainManifests_WebhookWithTargetNamespaces(t *testing.T) {
	sideEffects := admissionregistrationv1.SideEffectClassNone

	t.Run("validating webhook with specific target namespace sets namespace selector", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeSingleNamespace).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
							},
						},
					},
				}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "validate.foo",
					DeploymentName:          "my-controller",
					ContainerPort:           443,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "install-ns", registryv1.WithTargetNamespaces("watch-ns"))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "ValidatingWebhookConfiguration")
	})

	t.Run("mutating webhook with specific target namespace sets namespace selector", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeSingleNamespace).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
							},
						},
					},
				}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.MutatingAdmissionWebhook,
					GenerateName:            "mutate.foo",
					DeploymentName:          "my-controller",
					ContainerPort:           443,
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "install-ns", registryv1.WithTargetNamespaces("watch-ns"))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "MutatingWebhookConfiguration")
	})
}

func TestToPlainManifests_MultiNamespace(t *testing.T) {
	t.Run("MultiNamespace generates roles per target namespace", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeMultiNamespace).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
							},
						},
					},
				}).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "install-ns", registryv1.WithTargetNamespaces("ns1", "ns2"))
		require.NoError(t, err)
		// Should generate Role+RoleBinding per target namespace
		roles := findAllObjectsOfKind(objs, "Role")
		assert.Len(t, roles, 2)
		roleBindings := findAllObjectsOfKind(objs, "RoleBinding")
		assert.Len(t, roleBindings, 2)
	})
}

func TestToPlainManifests_ServiceAccountHandling(t *testing.T) {
	t.Run("default service account is not generated", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertNoObjectOfKind(t, objs, "ServiceAccount")
	})

	t.Run("named service account is generated", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "my-custom-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
					},
				}).
				Build(),
		}

		objs, err := registryv1.ToPlainManifests(b, "my-namespace")
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "ServiceAccount")
	})
}

func TestToPlainManifests_AdditionalValidation(t *testing.T) {
	t.Run("rejects duplicate webhook names", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "dep"}).
				WithWebhookDefinitions(
					v1alpha1.WebhookDescription{
						Type:                    v1alpha1.ValidatingAdmissionWebhook,
						GenerateName:            "dup-webhook",
						DeploymentName:          "dep",
						SideEffects:             &sideEffects,
						AdmissionReviewVersions: []string{"v1"},
					},
					v1alpha1.WebhookDescription{
						Type:                    v1alpha1.ValidatingAdmissionWebhook,
						GenerateName:            "dup-webhook",
						DeploymentName:          "dep",
						SideEffects:             &sideEffects,
						AdmissionReviewVersions: []string{"v1"},
					},
				).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate webhook")
	})

	t.Run("rejects duplicate CRDs", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"}},
			},
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate custom resource definition")
	})

	t.Run("rejects webhook with forbidden API group", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "dep"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "bad-webhook",
					DeploymentName:          "dep",
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
					Rules: []admissionregistrationv1.RuleWithOperations{
						{Rule: admissionregistrationv1.Rule{APIGroups: []string{"*"}, Resources: []string{"pods"}}},
					},
				}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "forbidden rule")
	})

	t.Run("rejects conversion webhook CRD referenced by multiple webhooks", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "dep"}).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				WithWebhookDefinitions(
					v1alpha1.WebhookDescription{
						Type: v1alpha1.ConversionWebhook, GenerateName: "conv1",
						DeploymentName: "dep", ConversionCRDs: []string{"foos.example.com"},
						AdmissionReviewVersions: []string{"v1"},
					},
					v1alpha1.WebhookDescription{
						Type: v1alpha1.ConversionWebhook, GenerateName: "conv2",
						DeploymentName: "dep", ConversionCRDs: []string{"foos.example.com"},
						AdmissionReviewVersions: []string{"v1"},
					},
				).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"}},
			},
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
	})

	t.Run("rejects conversion webhook referencing non-owned CRD", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "dep"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type: v1alpha1.ConversionWebhook, GenerateName: "conv",
					DeploymentName: "dep", ConversionCRDs: []string{"not-owned.example.com"},
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not owned")
	})
}

func TestToPlainManifests_WebhookWithCustomContainerPort(t *testing.T) {
	sideEffects := admissionregistrationv1.SideEffectClassNone
	b := registryv1.Bundle{
		PackageName: "my-operator",
		CSV: clusterserviceversion.Builder().
			WithName("my-operator.v1.0.0").
			WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
			WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
				Name: "my-controller",
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
						},
					},
				},
			}).
			WithWebhookDefinitions(v1alpha1.WebhookDescription{
				Type:                    v1alpha1.ValidatingAdmissionWebhook,
				GenerateName:            "validate.foo",
				DeploymentName:          "my-controller",
				ContainerPort:           9443,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
				WebhookPath:             ptr.To("/validate"),
			}).
			Build(),
	}

	objs, err := registryv1.ToPlainManifests(b, "ns")
	require.NoError(t, err)
	assertHasObjectOfKind(t, objs, "Service")
	assertHasObjectOfKind(t, objs, "ValidatingWebhookConfiguration")
}

func TestToPlainManifests_ConversionWebhookWithCertProvider(t *testing.T) {
	t.Run("cert-manager with conversion webhook generates cert resources and modifies CRD", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
							},
						},
					},
				}).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ConversionWebhook,
					GenerateName:            "convert.foos",
					DeploymentName:          "my-controller",
					ContainerPort:           443,
					ConversionCRDs:          []string{"foos.example.com"},
					AdmissionReviewVersions: []string{"v1"},
					WebhookPath:             ptr.To("/convert"),
				}).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "CustomResourceDefinition", APIVersion: apiextensionsv1.SchemeGroupVersion.String()},
					ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"},
				},
			},
		}

		provider := &certproviders.CertManagerCertificateProvider{}
		objs, err := registryv1.ToPlainManifests(b, "ns", registryv1.WithCertificateProvider(provider))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "Issuer")
		assertHasObjectOfKind(t, objs, "Certificate")
		assertHasObjectOfKind(t, objs, "CustomResourceDefinition")

		crd := findObjectByKind(objs, "CustomResourceDefinition")
		require.NotNil(t, crd)
		require.Contains(t, crd.GetAnnotations(), "cert-manager.io/inject-ca-from")
	})

	t.Run("openshift service CA with conversion webhook injects CA into CRD", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "my-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "img:latest"}},
							},
						},
					},
				}).
				WithOwnedCRDs(v1alpha1.CRDDescription{Name: "foos.example.com"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ConversionWebhook,
					GenerateName:            "convert.foos",
					DeploymentName:          "my-controller",
					ContainerPort:           443,
					ConversionCRDs:          []string{"foos.example.com"},
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
			CRDs: []apiextensionsv1.CustomResourceDefinition{
				{
					TypeMeta:   metav1.TypeMeta{Kind: "CustomResourceDefinition", APIVersion: apiextensionsv1.SchemeGroupVersion.String()},
					ObjectMeta: metav1.ObjectMeta{Name: "foos.example.com"},
				},
			},
		}

		provider := &certproviders.OpenshiftServiceCaCertificateProvider{}
		objs, err := registryv1.ToPlainManifests(b, "ns", registryv1.WithCertificateProvider(provider))
		require.NoError(t, err)
		assertHasObjectOfKind(t, objs, "CustomResourceDefinition")

		crd := findObjectByKind(objs, "CustomResourceDefinition")
		require.NotNil(t, crd)
		require.Contains(t, crd.GetAnnotations(), "service.beta.openshift.io/inject-cabundle")
	})
}

func TestFromFS_ErrorPaths(t *testing.T) {
	t.Run("fails on subdirectory in manifests", func(t *testing.T) {
		fs := fstest.MapFS{
			"metadata/annotations.yaml":  &fstest.MapFile{Data: []byte("annotations:\n  operators.operatorframework.io.bundle.package.v1: test\n")},
			"manifests/subdir/file.yaml": &fstest.MapFile{Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n")},
		}
		_, err := registryv1.FromFS(fs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "subdirectories are not allowed")
	})

	t.Run("fails on invalid YAML in manifests", func(t *testing.T) {
		fs := fstest.MapFS{
			"metadata/annotations.yaml": &fstest.MapFile{Data: []byte("annotations:\n  operators.operatorframework.io.bundle.package.v1: test\n")},
			"manifests/bad.yaml":        &fstest.MapFile{Data: []byte("not: valid: yaml: {{{}}")},
		}
		_, err := registryv1.FromFS(fs)
		require.Error(t, err)
	})

	t.Run("fails on invalid annotations.yaml", func(t *testing.T) {
		fs := fstest.MapFS{
			"metadata/annotations.yaml": &fstest.MapFile{Data: []byte("{{invalid yaml")},
		}
		_, err := registryv1.FromFS(fs)
		require.Error(t, err)
	})

	t.Run("parses bundle without properties.yaml", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("test").
			WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
				WithName("test.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build())).
			Build()
		// Remove properties file if present
		delete(fs, "metadata/properties.yaml")

		b, err := registryv1.FromFS(fs)
		require.NoError(t, err)
		assert.Equal(t, "test", b.PackageName)
	})
}

func TestToPlainManifests_MoreValidation(t *testing.T) {
	t.Run("rejects invalid webhook name", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "dep"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "INVALID_WEBHOOK_NAME!!",
					DeploymentName:          "dep",
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
				}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid name")
	})

	t.Run("rejects invalid deployment name", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "INVALID_DEP!!"}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid")
	})

	t.Run("rejects webhook rule with admissionregistration.k8s.io resource", func(t *testing.T) {
		sideEffects := admissionregistrationv1.SideEffectClassNone
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{Name: "dep"}).
				WithWebhookDefinitions(v1alpha1.WebhookDescription{
					Type:                    v1alpha1.ValidatingAdmissionWebhook,
					GenerateName:            "bad-rule-webhook",
					DeploymentName:          "dep",
					SideEffects:             &sideEffects,
					AdmissionReviewVersions: []string{"v1"},
					Rules: []admissionregistrationv1.RuleWithOperations{
						{Rule: admissionregistrationv1.Rule{
							APIGroups: []string{"admissionregistration.k8s.io"},
							Resources: []string{"validatingwebhookconfigurations"},
						}},
					},
				}).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns")
		require.Error(t, err)
		require.Contains(t, err.Error(), "forbidden rule")
	})

	t.Run("rejects OwnNamespace target when not supported", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns", registryv1.WithTargetNamespaces("ns"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "do not support targeting own namespace")
	})

	t.Run("rejects multi-namespace target when not supported", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns", registryv1.WithTargetNamespaces("a", "b"))
		require.Error(t, err)
	})

	t.Run("rejects OwnNamespace in multi-namespace when not supported", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeMultiNamespace).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns", registryv1.WithTargetNamespaces("ns", "other"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "do not support targeting own namespace")
	})

	t.Run("rejects SingleNamespace target as own namespace", func(t *testing.T) {
		b := registryv1.Bundle{
			PackageName: "my-operator",
			CSV: clusterserviceversion.Builder().
				WithName("my-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeOwnNamespace).
				Build(),
		}
		_, err := registryv1.ToPlainManifests(b, "ns", registryv1.WithTargetNamespaces("different-ns"))
		require.Error(t, err)
	})
}

func TestFromFSThenToPlainManifests(t *testing.T) {
	t.Run("end-to-end: parse then render", func(t *testing.T) {
		fs := bundlefs.Builder().
			WithPackageName("e2e-operator").
			WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
				WithName("e2e-operator.v1.0.0").
				WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
				WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
					Name: "e2e-controller",
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "manager", Image: "e2e:latest"}},
							},
						},
					},
				}).
				WithPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "e2e-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list"}},
					},
				}).
				WithClusterPermissions(v1alpha1.StrategyDeploymentPermissions{
					ServiceAccountName: "e2e-sa",
					Rules: []rbacv1.PolicyRule{
						{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"get", "list"}},
					},
				}).
				Build())).
			Build()

		b, err := registryv1.FromFS(fs)
		require.NoError(t, err)

		objs, err := registryv1.ToPlainManifests(b, "operator-ns")
		require.NoError(t, err)
		require.NotEmpty(t, objs)

		assertHasObjectOfKind(t, objs, "ServiceAccount")
		assertHasObjectOfKind(t, objs, "ClusterRole")
		assertHasObjectOfKind(t, objs, "ClusterRoleBinding")
		assertHasObjectOfKind(t, objs, "Deployment")
	})
}

// --- Test helpers ---

func findObjectByKind(objs []client.Object, kind string) client.Object {
	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().Kind == kind {
			return obj
		}
	}
	return nil
}

func findObjectByKindAndName(objs []client.Object, kind, name string) client.Object {
	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().Kind == kind && obj.GetName() == name {
			return obj
		}
	}
	return nil
}

func findAllObjectsOfKind(objs []client.Object, kind string) []client.Object {
	var result []client.Object
	for _, obj := range objs {
		if obj.GetObjectKind().GroupVersionKind().Kind == kind {
			result = append(result, obj)
		}
	}
	return result
}

func assertHasObjectOfKind(t *testing.T, objs []client.Object, kind string) {
	t.Helper()
	assert.NotNilf(t, findObjectByKind(objs, kind), "expected to find object of kind %q", kind)
}

func assertHasObjectOfKindAndName(t *testing.T, objs []client.Object, kind, name string) {
	t.Helper()
	assert.NotNilf(t, findObjectByKindAndName(objs, kind, name), "expected to find object of kind %q with name %q", kind, name)
}

func assertNoObjectOfKind(t *testing.T, objs []client.Object, kind string) {
	t.Helper()
	assert.Nilf(t, findObjectByKind(objs, kind), "expected no object of kind %q", kind)
}
