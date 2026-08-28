package generators

import (
	"errors"
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing/clusterserviceversion"
)

func csvWithAnnotations(annotations map[string]string) v1alpha1.ClusterServiceVersion {
	return v1alpha1.ClusterServiceVersion{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
		},
	}
}

func Test_resolveInstallNamespace_SelfManaged(t *testing.T) {
	name := "caller-managed"
	rv1 := &bundle.RegistryV1{PackageName: "my-package"}

	ns, emit, err := resolveInstallNamespace(rv1, &name)
	require.NoError(t, err)
	assert.Equal(t, "caller-managed", ns.Name)
	assert.False(t, emit, "self-managed namespace must not emit a Namespace object")
}

func Test_resolveInstallNamespace_SelfManagedInvalid(t *testing.T) {
	name := "Invalid_Name"
	_, _, err := resolveInstallNamespace(&bundle.RegistryV1{}, &name)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-managed install namespace")
}

func Test_resolveInstallNamespace_Template(t *testing.T) {
	tmpl := `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"tmpl-ns","labels":{"pod-security.kubernetes.io/enforce":"restricted"},"annotations":{"foo":"bar"}}}`
	rv1 := &bundle.RegistryV1{
		PackageName: "my-package",
		CSV: csvWithAnnotations(map[string]string{
			annotationSuggestedNamespaceTemplate: tmpl,
			// suggested-namespace is present but must be ignored when template is set
			annotationSuggestedNamespace: "ignored-ns",
		}),
	}

	ns, emit, err := resolveInstallNamespace(rv1, nil)
	require.NoError(t, err)
	assert.True(t, emit)
	assert.Equal(t, "tmpl-ns", ns.Name)
	assert.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/enforce"])
	assert.Equal(t, "bar", ns.Annotations["foo"])
	assert.Equal(t, "Namespace", ns.Kind)
	assert.Equal(t, "v1", ns.APIVersion)
}

func Test_resolveInstallNamespace_TemplateEmptyNameErrors(t *testing.T) {
	rv1 := &bundle.RegistryV1{
		PackageName: "my-package",
		CSV: csvWithAnnotations(map[string]string{
			annotationSuggestedNamespaceTemplate: `{"apiVersion":"v1","kind":"Namespace","metadata":{"labels":{"a":"b"}}}`,
			annotationSuggestedNamespace:         "fallback-ns",
		}),
	}

	_, _, err := resolveInstallNamespace(rv1, nil)
	require.Error(t, err, "empty template name must error, not fall through")
	assert.Contains(t, err.Error(), "install namespace")
}

func Test_resolveInstallNamespace_TemplateMalformed(t *testing.T) {
	rv1 := &bundle.RegistryV1{
		CSV: csvWithAnnotations(map[string]string{
			annotationSuggestedNamespaceTemplate: `{not valid`,
		}),
	}

	_, _, err := resolveInstallNamespace(rv1, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), annotationSuggestedNamespaceTemplate)
}

func Test_resolveInstallNamespace_SuggestedNamespace(t *testing.T) {
	rv1 := &bundle.RegistryV1{
		PackageName: "my-package",
		CSV: csvWithAnnotations(map[string]string{
			annotationSuggestedNamespace: "suggested-ns",
		}),
	}

	ns, emit, err := resolveInstallNamespace(rv1, nil)
	require.NoError(t, err)
	assert.True(t, emit)
	assert.Equal(t, "suggested-ns", ns.Name)
	assert.Empty(t, ns.Labels)
	assert.Empty(t, ns.Annotations)
	assert.Equal(t, "Namespace", ns.Kind)
}

func Test_resolveInstallNamespace_SuggestedNamespaceEmptyErrors(t *testing.T) {
	rv1 := &bundle.RegistryV1{
		PackageName: "my-package",
		CSV: csvWithAnnotations(map[string]string{
			annotationSuggestedNamespace: "",
		}),
	}

	_, _, err := resolveInstallNamespace(rv1, nil)
	require.Error(t, err, "present-but-empty suggested-namespace must error, not fall through to fallback")
}

func Test_resolveInstallNamespace_Fallback(t *testing.T) {
	rv1 := &bundle.RegistryV1{PackageName: "my-package"}

	ns, emit, err := resolveInstallNamespace(rv1, nil)
	require.NoError(t, err)
	assert.True(t, emit)
	assert.Equal(t, "my-package-system", ns.Name)
}

func Test_validateNamespaceName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "my-namespace", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "too long", input: strings.Repeat("a", 64), wantErr: true},
		{name: "max length ok", input: strings.Repeat("a", 63), wantErr: false},
		{name: "uppercase", input: "MyNamespace", wantErr: true},
		{name: "underscore", input: "my_namespace", wantErr: true},
		{name: "leading dash", input: "-ns", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNamespaceName(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_validateTargetNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		supportedInstallModes []v1alpha1.InstallModeType
		targetNamespaces      []string
		err                   error
	}{
		{
			name: "rejects all namespace install if AllNamespaces install mode is not supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
			},
			targetNamespaces: []string{corev1.NamespaceAll},
			err:              errors.New("supported install modes [SingleNamespace] do not support targeting all namespaces"),
		},
		{
			name: "rejects own namespace install if only AllNamespaces install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeAllNamespaces,
			},
			targetNamespaces: []string{"install-namespace"},
			err:              errors.New("supported install modes [AllNamespaces] do not support targeting own namespace"),
		},
		{
			name: "rejects own namespace install if only SingleNamespace install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
			},
			targetNamespaces: []string{"install-namespace"},
			err:              errors.New("supported install modes [SingleNamespace] do not support targeting own namespace"),
		},
		{
			name: "rejects install out of own namespace if only OwnNamespace install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeOwnNamespace,
			},
			targetNamespaces: []string{"not-install-namespace"},
			err:              errors.New("supported install modes [OwnNamespace] do not support target namespaces [not-install-namespace]"),
		},
		{
			name: "rejects multi-namespace install if MultiNamespace install mode is not supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeAllNamespaces,
			},
			targetNamespaces: []string{"ns1", "ns2", "ns3"},
			err:              errors.New("supported install modes [AllNamespaces] do not support target namespaces [ns1 ns2 ns3]"),
		},
		{
			name:             "rejects if bundle supports no install modes",
			targetNamespaces: []string{"some-namespace"},
			err:              errors.New("supported install modes [] do not support target namespaces [some-namespace]"),
		},
		{
			name: "accepts all namespace render if AllNamespaces install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeAllNamespaces,
			},
			targetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name: "accepts install namespace render if OwnNamespace install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeOwnNamespace,
			},
			targetNamespaces: []string{"install-namespace"},
		},
		{
			name: "accepts single namespace render if SingleNamespace install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
			},
			targetNamespaces: []string{"some-namespace"},
		},
		{
			name: "accepts multi namespace render if MultiNamespace install mode is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeMultiNamespace,
			},
			targetNamespaces: []string{"n1", "n2", "n3"},
		},
		{
			name: "rejects multi-namespace render if OwnNamespace install mode is not supported and target namespaces include install namespace",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeMultiNamespace,
			},
			targetNamespaces: []string{"n1", "n2", "n3", "install-namespace"},
			err:              errors.New("supported install modes [MultiNamespace] do not support targeting own namespace"),
		},
		{
			name: "rejects missing target namespaces when only SingleNamespace is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
			},
			err: errors.New("exactly one target namespace must be specified"),
		},
		{
			name: "rejects missing target namespaces when only OwnNamespace is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeOwnNamespace,
			},
			err: errors.New("exactly one target namespace must be specified"),
		},
		{
			name: "rejects missing target namespaces when MultiNamespace is supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeMultiNamespace,
			},
			err: errors.New("at least one target namespace must be specified"),
		},
		{
			name: "rejects missing target namespaces when SingleNamespace and OwnNamespace are supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
				v1alpha1.InstallModeTypeOwnNamespace,
			},
			err: errors.New("exactly one target namespace must be specified"),
		},
		{
			name: "rejects missing target namespaces when SingleNamespace and MultiNamespace are supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
				v1alpha1.InstallModeTypeMultiNamespace,
			},
			err: errors.New("at least one target namespace must be specified"),
		},
		{
			name: "rejects missing target namespaces when OwnNamespace and MultiNamespace are supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeOwnNamespace,
				v1alpha1.InstallModeTypeMultiNamespace,
			},
			err: errors.New("at least one target namespace must be specified"),
		},
		{
			name: "rejects missing target namespaces when SingleNamespace, OwnNamespace, and MultiNamespace are supported",
			supportedInstallModes: []v1alpha1.InstallModeType{
				v1alpha1.InstallModeTypeSingleNamespace,
				v1alpha1.InstallModeTypeOwnNamespace,
				v1alpha1.InstallModeTypeMultiNamespace,
			},
			err: errors.New("at least one target namespace must be specified"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installNamespace := "install-namespace"
			err := validateTargetNamespaces(
				&bundle.RegistryV1{
					CSV: clusterserviceversion.Builder().
						WithInstallModeSupportFor(tc.supportedInstallModes...).
						Build(),
				},
				installNamespace,
				tc.targetNamespaces,
			)
			if tc.err == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.err.Error())
			}
		})
	}
}

func Test_BundleNamespaceGenerator(t *testing.T) {
	t.Run("sets self-managed namespace without emitting an object", func(t *testing.T) {
		selfManagedNamespace := "caller-managed"
		ctx := &render.GeneratorContext{}

		err := BundleNamespaceGenerator(
			bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().
					WithInstallModeSupportFor(v1alpha1.InstallModeTypeOwnNamespace).
					Build(),
			},
			render.Options{
				SelfManagedInstallNamespace: &selfManagedNamespace,
				TargetNamespaces:            []string{selfManagedNamespace},
			},
			ctx,
		)

		require.NoError(t, err)
		assert.Equal(t, selfManagedNamespace, ctx.InstallNamespace)
		assert.Empty(t, ctx.Objects)
	})

	t.Run("sets suggested namespace template and emits its metadata", func(t *testing.T) {
		ctx := &render.GeneratorContext{}
		err := BundleNamespaceGenerator(
			bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().
					WithAnnotations(map[string]string{
						annotationSuggestedNamespaceTemplate: `{"metadata":{"name":"template-namespace","labels":{"foo":"bar"},"annotations":{"example.com/annotation":"value"}}}`,
					}).
					WithInstallModeSupportFor(v1alpha1.InstallModeTypeOwnNamespace).
					Build(),
			},
			render.Options{
				TargetNamespaces: []string{"template-namespace"},
			},
			ctx,
		)

		require.NoError(t, err)
		assert.Equal(t, "template-namespace", ctx.InstallNamespace)
		require.Len(t, ctx.Objects, 1)
		assert.Equal(t, "template-namespace", ctx.Objects[0].GetName())
		assert.Equal(t, map[string]string{"foo": "bar"}, ctx.Objects[0].GetLabels())
		assert.Equal(t, map[string]string{"example.com/annotation": "value"}, ctx.Objects[0].GetAnnotations())
	})

	t.Run("sets suggested namespace and emits a bare namespace", func(t *testing.T) {
		ctx := &render.GeneratorContext{}
		err := BundleNamespaceGenerator(
			bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().
					WithAnnotations(map[string]string{
						annotationSuggestedNamespace: "suggested-namespace",
					}).
					WithInstallModeSupportFor(v1alpha1.InstallModeTypeOwnNamespace).
					Build(),
			},
			render.Options{
				TargetNamespaces: []string{"suggested-namespace"},
			},
			ctx,
		)

		require.NoError(t, err)
		assert.Equal(t, "suggested-namespace", ctx.InstallNamespace)
		require.Len(t, ctx.Objects, 1)
		assert.Equal(t, newNamespace("suggested-namespace"), ctx.Objects[0])
	})

	t.Run("leaves context unchanged when self-managed namespace is invalid", func(t *testing.T) {
		invalidNamespace := "Invalid_Name"
		ctx := &render.GeneratorContext{
			InstallNamespace: "existing-namespace",
			Objects:          []client.Object{newNamespace("existing-namespace")},
		}
		expectedCtx := *ctx

		err := BundleNamespaceGenerator(
			bundle.RegistryV1{},
			render.Options{SelfManagedInstallNamespace: &invalidNamespace},
			ctx,
		)

		require.Error(t, err)
		assert.Equal(t, expectedCtx, *ctx)
	})

	t.Run("leaves context unchanged when target namespaces are invalid", func(t *testing.T) {
		ctx := &render.GeneratorContext{}

		err := BundleNamespaceGenerator(
			bundle.RegistryV1{
				PackageName: "my-package",
				CSV: clusterserviceversion.Builder().
					WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
					Build(),
			},
			render.Options{TargetNamespaces: []string{"target-namespace"}},
			ctx,
		)

		require.EqualError(t, err, "invalid option(s): invalid target namespaces [target-namespace]: supported install modes [AllNamespaces] do not support target namespaces [target-namespace]")
		assert.Empty(t, ctx.InstallNamespace)
		assert.Empty(t, ctx.Objects)
	})

	t.Run("sets derived namespace from package-name fallback and emits it", func(t *testing.T) {
		ctx := &render.GeneratorContext{}
		err := BundleNamespaceGenerator(
			bundle.RegistryV1{
				PackageName: "my-package",
				CSV: clusterserviceversion.Builder().
					WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
					Build(),
			},
			render.Options{TargetNamespaces: []string{corev1.NamespaceAll}},
			ctx,
		)

		require.NoError(t, err)
		assert.Equal(t, "my-package-system", ctx.InstallNamespace)
		require.Len(t, ctx.Objects, 1)
		assert.Equal(t, newNamespace("my-package-system"), ctx.Objects[0])
	})
}
