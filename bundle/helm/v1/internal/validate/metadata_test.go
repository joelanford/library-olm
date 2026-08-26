package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestHelmMetadata(t *testing.T) {
	tests := []struct {
		name       string
		objects    []client.Object
		wantErrors []string
	}{
		{
			name: "empty metadata",
			objects: []client.Object{&unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "example"},
			}}},
		},
		{
			name: "allowed Helm label",
			objects: []client.Object{metadataObject(map[string]string{
				"helm.sh/chart": "example-1.0.0",
			}, nil)},
		},
		{
			name: "non Helm metadata",
			objects: []client.Object{metadataObject(map[string]string{
				"app.kubernetes.io/name": "example",
			}, map[string]string{
				"example.com/owner": "team",
			})},
		},
		{
			name: "disallowed annotation",
			objects: []client.Object{metadataObject(nil, map[string]string{
				"helm.sh/hook": "pre-install",
			})},
			wantErrors: []string{"example", "helm.sh/hook"},
		},
		{
			name: "disallowed label",
			objects: []client.Object{metadataObject(map[string]string{
				"helm.sh/unknown": "value",
			}, nil)},
			wantErrors: []string{"example", "helm.sh/unknown"},
		},
		{
			name: "violations across objects",
			objects: []client.Object{
				metadataObject(nil, map[string]string{"helm.sh/unknown-annotation": "value"}),
				metadataObject(map[string]string{"helm.sh/unknown-label": "value"}, nil),
			},
			wantErrors: []string{"helm.sh/unknown-label", "helm.sh/unknown-annotation"},
		},
		{
			name: "typed object",
			objects: []client.Object{&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name:        "example",
				Annotations: map[string]string{"helm.sh/hook": "pre-install"},
			}}},
			wantErrors: []string{"example", "helm.sh/hook"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := HelmMetadata(test.objects)
			if len(test.wantErrors) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, wantError := range test.wantErrors {
				require.ErrorContains(t, err, wantError)
			}
		})
	}
}

func metadataObject(labels, annotations map[string]string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "example",
		},
	}}
	object.SetLabels(labels)
	object.SetAnnotations(annotations)
	return object
}
