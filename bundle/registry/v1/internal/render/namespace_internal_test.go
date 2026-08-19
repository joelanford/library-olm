package render

import (
	"strings"
	"testing"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
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
