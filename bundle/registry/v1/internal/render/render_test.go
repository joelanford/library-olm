package render_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/config"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render"
	. "github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing/clusterserviceversion"
)

func Test_BundleRenderer_NoConfig(t *testing.T) {
	renderer := render.BundleRenderer{}
	objs, err := renderer.Render(
		bundle.RegistryV1{
			CSV: clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
		}, render.WithSelfManagedInstallNamespace("install-namespace"), nil)
	require.NoError(t, err)
	require.Empty(t, objs)
}

func Test_BundleRenderer_ValidatesBundle(t *testing.T) {
	renderer := render.BundleRenderer{
		BundleValidator: render.BundleValidator{
			func(v1 *bundle.RegistryV1) []error {
				return []error{errors.New("this bundle is invalid")}
			},
		},
	}
	objs, err := renderer.Render(bundle.RegistryV1{}, render.WithSelfManagedInstallNamespace("install-namespace"))
	require.Nil(t, objs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "this bundle is invalid")
}

func Test_BundleRenderer_CreatesCorrectDefaultOptions(t *testing.T) {
	expectedTargetNamespaces := []string{""}
	expectedUniqueNameGenerator := render.DefaultUniqueNameGenerator
	sentinelErr := errors.New("sentinel")

	renderer := render.BundleRenderer{
		ResourceGenerators: []render.ResourceGenerator{
			func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
				require.Nil(t, opts.SelfManagedInstallNamespace)
				require.Equal(t, expectedTargetNamespaces, opts.TargetNamespaces)
				require.Equal(t, reflect.ValueOf(expectedUniqueNameGenerator).Pointer(), reflect.ValueOf(render.DefaultUniqueNameGenerator).Pointer(), "options has unexpected default unique name generator")
				return sentinelErr
			},
		},
	}

	_, err := renderer.Render(bundle.RegistryV1{
		PackageName: "default-operator",
		CSV:         clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
	})
	require.Equal(t, sentinelErr, err, "did not get sentinel error, generator assertions did not run")
}

func Test_BundleRenderer_DefaultTargetNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		supportedInstallModes    []v1alpha1.InstallModeType
		expectedTargetNamespaces []string
	}{
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, OwnNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeOwnNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, SingleNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeSingleNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, MultiNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeMultiNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, OwnNamespace, SingleNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeOwnNamespace, v1alpha1.InstallModeTypeSingleNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, OwnNamespace, MultiNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeOwnNamespace, v1alpha1.InstallModeTypeMultiNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, SingleNamespace, MultiNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeSingleNamespace, v1alpha1.InstallModeTypeMultiNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
		{
			name:                     "Default to AllNamespaces when bundle install modes are {AllNamespaces, SingleNamespace, OwnNamespace, MultiNamespace}",
			supportedInstallModes:    []v1alpha1.InstallModeType{v1alpha1.InstallModeTypeAllNamespaces, v1alpha1.InstallModeTypeSingleNamespace, v1alpha1.InstallModeTypeOwnNamespace, v1alpha1.InstallModeTypeMultiNamespace},
			expectedTargetNamespaces: []string{corev1.NamespaceAll},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			renderer := render.BundleRenderer{
				ResourceGenerators: []render.ResourceGenerator{
					func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
						require.Equal(t, tc.expectedTargetNamespaces, opts.TargetNamespaces)
						return nil
					},
				},
			}
			_, err := renderer.Render(bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().
					WithName("test").
					WithInstallModeSupportFor(tc.supportedInstallModes...).Build(),
			}, render.WithSelfManagedInstallNamespace("some-namespace"))
			require.NoError(t, err)
		})
	}
}

func Test_BundleRenderer_RejectsNilUniqueNameGenerator(t *testing.T) {
	_, err := render.BundleRenderer{}.Render(
		bundle.RegistryV1{},
		render.WithUniqueNameGenerator(nil),
	)
	require.EqualError(t, err, "invalid option(s): unique name generator must be specified")
}

func Test_BundleRenderer_AppliesUserOptions(t *testing.T) {
	isOptionApplied := false
	_, _ = render.BundleRenderer{}.Render(bundle.RegistryV1{}, render.WithSelfManagedInstallNamespace("install-namespace"), func(options *render.Options) {
		isOptionApplied = true
	})
	require.True(t, isOptionApplied)
}

func Test_WithTargetNamespaces(t *testing.T) {
	t.Run("sets target namespaces when provided", func(t *testing.T) {
		opts := &render.Options{
			TargetNamespaces: []string{"target-namespace"},
		}
		render.WithTargetNamespaces("a", "b", "c")(opts)
		require.Equal(t, []string{"a", "b", "c"}, opts.TargetNamespaces)
	})

	t.Run("preserves the default when empty", func(t *testing.T) {
		opts := &render.Options{
			TargetNamespaces: []string{"target-namespace"},
		}
		render.WithTargetNamespaces()(opts)
		require.Equal(t, []string{"target-namespace"}, opts.TargetNamespaces)
	})
}

func Test_WithUniqueNameGenerator(t *testing.T) {
	opts := &render.Options{
		UniqueNameGenerator: render.DefaultUniqueNameGenerator,
	}
	render.WithUniqueNameGenerator(func(s string, i interface{}) string {
		return "a man needs a name"
	})(opts)
	generatedName := opts.UniqueNameGenerator("", nil)
	require.Equal(t, "a man needs a name", generatedName)
}

func Test_WithCertificateProvide(t *testing.T) {
	opts := &render.Options{}
	expectedCertProvider := FakeCertProvider{}
	render.WithCertificateProvider(expectedCertProvider)(opts)
	require.Equal(t, expectedCertProvider, opts.CertificateProvider)
}

func Test_BundleRenderer_CallsResourceGenerators(t *testing.T) {
	renderer := render.BundleRenderer{
		ResourceGenerators: []render.ResourceGenerator{
			func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
				ctx.Objects = append(ctx.Objects, &corev1.Namespace{}, &corev1.Service{})
				return nil
			},
			func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
				ctx.Objects = append(ctx.Objects, &appsv1.Deployment{})
				return nil
			},
		},
	}
	objs, err := renderer.Render(
		bundle.RegistryV1{
			CSV: clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
		}, render.WithSelfManagedInstallNamespace("install-namespace"))
	require.NoError(t, err)
	require.Equal(t, []client.Object{&corev1.Namespace{}, &corev1.Service{}, &appsv1.Deployment{}}, objs)
}

func Test_BundleRenderer_ReturnsResourceGeneratorErrors(t *testing.T) {
	renderer := render.BundleRenderer{
		ResourceGenerators: []render.ResourceGenerator{
			func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
				ctx.Objects = append(ctx.Objects, &corev1.Namespace{}, &corev1.Service{})
				return nil
			},
			func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
				return fmt.Errorf("generator error")
			},
		},
	}
	objs, err := renderer.Render(
		bundle.RegistryV1{
			CSV: clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
		}, render.WithSelfManagedInstallNamespace("install-namespace"))
	require.Nil(t, objs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "generator error")
}

func Test_BundleValidatorCallsAllValidationFnsInOrder(t *testing.T) {
	actual := ""
	val := render.BundleValidator{
		func(v1 *bundle.RegistryV1) []error {
			actual += "h"
			return nil
		},
		func(v1 *bundle.RegistryV1) []error {
			actual += "i"
			return nil
		},
	}
	require.NoError(t, val.Validate(nil))
	require.Equal(t, "hi", actual)
}

func Test_WithDeploymentConfig(t *testing.T) {
	t.Run("sets deployment config when provided", func(t *testing.T) {
		expectedConfig := &config.DeploymentConfig{
			Env: []corev1.EnvVar{
				{Name: "TEST_ENV", Value: "test-value"},
			},
		}

		var receivedConfig *config.DeploymentConfig
		renderer := render.BundleRenderer{
			ResourceGenerators: []render.ResourceGenerator{
				func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
					receivedConfig = opts.DeploymentConfig
					return nil
				},
			},
		}

		_, err := renderer.Render(
			bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
			},
			render.WithSelfManagedInstallNamespace("test-namespace"),
			render.WithDeploymentConfig(expectedConfig),
		)

		require.NoError(t, err)
		require.Equal(t, expectedConfig, receivedConfig)
	})

	t.Run("deployment config is nil when not provided", func(t *testing.T) {
		var receivedConfig *config.DeploymentConfig
		renderer := render.BundleRenderer{
			ResourceGenerators: []render.ResourceGenerator{
				func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
					receivedConfig = opts.DeploymentConfig
					return nil
				},
			},
		}

		_, err := renderer.Render(
			bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
			},
			render.WithSelfManagedInstallNamespace("test-namespace"),
		)

		require.NoError(t, err)
		require.Nil(t, receivedConfig)
	})

	t.Run("deployment config is nil when explicitly set to nil", func(t *testing.T) {
		var receivedConfig *config.DeploymentConfig
		renderer := render.BundleRenderer{
			ResourceGenerators: []render.ResourceGenerator{
				func(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
					receivedConfig = opts.DeploymentConfig
					return nil
				},
			},
		}

		_, err := renderer.Render(
			bundle.RegistryV1{
				CSV: clusterserviceversion.Builder().WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).Build(),
			},
			render.WithSelfManagedInstallNamespace("test-namespace"),
			render.WithDeploymentConfig(nil),
		)

		require.NoError(t, err)
		require.Nil(t, receivedConfig)
	})
}
