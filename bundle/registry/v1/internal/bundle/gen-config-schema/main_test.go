package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getPackageDir(t *testing.T, pkgPath string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pkgPath)
	out, err := cmd.Output()
	require.NoError(t, err, "failed to find package %s", pkgPath)
	return strings.TrimSpace(string(out))
}

func getMockOpenAPISpec() *openAPISpec {
	return &openAPISpec{
		Components: struct {
			Schemas map[string]any `json:"schemas"`
		}{
			Schemas: map[string]any{
				"io.k8s.api.core.v1.Toleration": map[string]any{
					"type":        "object",
					"description": "The pod this Toleration is attached to tolerates any taint that matches the triple <key,value,effect> using the matching operator <operator>.",
					"properties": map[string]any{
						"key":               map[string]string{"type": "string"},
						"operator":          map[string]string{"type": "string"},
						"value":             map[string]string{"type": "string"},
						"effect":            map[string]string{"type": "string"},
						"tolerationSeconds": map[string]any{"type": "integer", "format": "int64"},
					},
				},
				"io.k8s.api.core.v1.ResourceRequirements": map[string]any{
					"type":        "object",
					"description": "ResourceRequirements describes the compute resource requirements.",
					"properties": map[string]any{
						"limits":   map[string]any{"type": "object"},
						"requests": map[string]any{"type": "object"},
					},
				},
				"io.k8s.api.core.v1.EnvVar": map[string]any{
					"type":       "object",
					"properties": map[string]any{"name": map[string]string{"type": "string"}},
				},
				"io.k8s.api.core.v1.EnvFromSource": map[string]any{
					"type": "object",
				},
				"io.k8s.api.core.v1.Volume": map[string]any{
					"type": "object",
				},
				"io.k8s.api.core.v1.VolumeMount": map[string]any{
					"type": "object",
				},
				"io.k8s.api.core.v1.Affinity": map[string]any{
					"type": "object",
				},
			},
		},
	}
}

func TestParseSubscriptionConfig(t *testing.T) {
	pkgDir := getPackageDir(t, "github.com/operator-framework/api/pkg/operators/v1alpha1")
	subscriptionTypesFile := filepath.Join(pkgDir, "subscription_types.go")

	fields, err := parseSubscriptionConfig(subscriptionTypesFile)
	require.NoError(t, err, "should successfully parse SubscriptionConfig")
	require.NotEmpty(t, fields, "should find fields in SubscriptionConfig")

	fieldMap := make(map[string]fieldInfo)
	for _, field := range fields {
		fieldMap[field.JSONName] = field
	}

	t.Run("includes expected fields", func(t *testing.T) {
		expectedFields := []string{
			"nodeSelector",
			"tolerations",
			"resources",
			"env",
			"envFrom",
			"volumes",
			"volumeMounts",
			"affinity",
			"annotations",
		}

		for _, fieldName := range expectedFields {
			assert.Contains(t, fieldMap, fieldName, "should include %s field", fieldName)
		}
	})

	t.Run("excludes selector field", func(t *testing.T) {
		assert.NotContains(t, fieldMap, "selector", "should exclude selector field")
	})

	t.Run("parses field types correctly", func(t *testing.T) {
		tolerations, ok := fieldMap["tolerations"]
		require.True(t, ok, "tolerations should be present")
		assert.True(t, tolerations.IsSlice, "tolerations should be a slice")
		assert.Equal(t, "corev1", tolerations.TypePkg, "tolerations should be from corev1 package")
		assert.Equal(t, "Toleration", tolerations.TypeName, "tolerations type should be Toleration")

		nodeSelector, ok := fieldMap["nodeSelector"]
		require.True(t, ok, "nodeSelector should be present")
		assert.True(t, nodeSelector.IsMap, "nodeSelector should be a map")

		resources, ok := fieldMap["resources"]
		require.True(t, ok, "resources should be present")
		assert.Equal(t, "corev1", resources.TypePkg)
		assert.Equal(t, "ResourceRequirements", resources.TypeName)
	})
}

func TestGenerateBundleConfigSchema(t *testing.T) {
	mockOpenAPI := getMockOpenAPISpec()

	fields := []fieldInfo{
		{JSONName: "nodeSelector", IsMap: true},
		{JSONName: "tolerations", TypePkg: "corev1", TypeName: "Toleration", IsSlice: true},
		{JSONName: "resources", TypePkg: "corev1", TypeName: "ResourceRequirements"},
		{JSONName: "annotations", IsMap: true},
	}

	s := generateBundleConfigSchema(mockOpenAPI, fields)

	t.Run("schema has correct metadata", func(t *testing.T) {
		assert.Equal(t, "http://json-schema.org/draft-07/schema#", s.Schema)
		assert.Equal(t, schemaID, s.ID)
		assert.Equal(t, schemaTitle, s.Title)
		assert.NotEmpty(t, s.Description)
		assert.Equal(t, "object", s.Type)
		assert.False(t, s.AdditionalProperties)
	})

	t.Run("includes watchNamespace property", func(t *testing.T) {
		require.Contains(t, s.Properties, "watchNamespace")

		watchNS := s.Properties["watchNamespace"]
		require.NotNil(t, watchNS)

		assert.NotEmpty(t, watchNS.Description)
		assert.Len(t, watchNS.AnyOf, 2, "watchNamespace should have anyOf with null and string")
	})

	t.Run("includes deploymentConfig property", func(t *testing.T) {
		require.Contains(t, s.Properties, "deploymentConfig")

		deployConfig := s.Properties["deploymentConfig"]
		require.NotNil(t, deployConfig)

		assert.Equal(t, "object", deployConfig.Type)
		assert.NotEmpty(t, deployConfig.Description)
		assert.Equal(t, false, deployConfig.AdditionalProperties)

		assert.Contains(t, deployConfig.Properties, "nodeSelector")
		assert.Contains(t, deployConfig.Properties, "tolerations")
		assert.Contains(t, deployConfig.Properties, "resources")
		assert.Contains(t, deployConfig.Properties, "annotations")
	})
}

func TestMapFieldToOpenAPISchema(t *testing.T) {
	mockOpenAPI := getMockOpenAPISpec()
	collector := &schemaCollector{
		openAPISpec:      mockOpenAPI,
		collectedSchemas: make(map[string]bool),
	}

	t.Run("maps map fields correctly", func(t *testing.T) {
		field := fieldInfo{
			JSONName: "nodeSelector",
			IsMap:    true,
		}

		s := mapFieldToOpenAPISchema(field, mockOpenAPI, collector)
		require.NotNil(t, s)

		assert.Equal(t, "object", s.Type)
		assert.NotNil(t, s.AdditionalProperties)
	})

	t.Run("maps slice fields correctly", func(t *testing.T) {
		field := fieldInfo{
			JSONName: "tolerations",
			TypePkg:  "corev1",
			TypeName: "Toleration",
			IsSlice:  true,
		}

		s := mapFieldToOpenAPISchema(field, mockOpenAPI, collector)
		require.NotNil(t, s)

		assert.Equal(t, "array", s.Type)
		assert.NotNil(t, s.Items)

		items, ok := s.Items.(*schemaField)
		require.True(t, ok)
		assert.Equal(t, "#/components/schemas/io.k8s.api.core.v1.Toleration", items.Ref)
	})

	t.Run("maps object fields correctly", func(t *testing.T) {
		field := fieldInfo{
			JSONName: "resources",
			TypePkg:  "corev1",
			TypeName: "ResourceRequirements",
		}

		s := mapFieldToOpenAPISchema(field, mockOpenAPI, collector)
		require.NotNil(t, s)

		assert.Equal(t, "#/components/schemas/io.k8s.api.core.v1.ResourceRequirements", s.Ref)
	})
}

func TestGetOpenAPITypeName(t *testing.T) {
	testCases := []struct {
		name     string
		field    fieldInfo
		expected string
	}{
		{
			name:     "corev1 package",
			field:    fieldInfo{TypePkg: "corev1", TypeName: "Toleration"},
			expected: "io.k8s.api.core.v1.Toleration",
		},
		{
			name:     "v1 package",
			field:    fieldInfo{TypePkg: "v1", TypeName: "ResourceRequirements"},
			expected: "io.k8s.api.core.v1.ResourceRequirements",
		},
		{
			name:     "unknown package",
			field:    fieldInfo{TypePkg: "unknown", TypeName: "SomeType"},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getOpenAPITypeName(tc.field)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCheckedInSchemaHasExpectedStructure(t *testing.T) {
	data, err := os.ReadFile("../registryv1bundleconfig.json")
	require.NoError(t, err, "should be able to read the checked-in schema file")

	var schemaFromFile map[string]any
	err = json.Unmarshal(data, &schemaFromFile)
	require.NoError(t, err, "checked-in schema should be valid JSON")

	assert.Equal(t, "http://json-schema.org/draft-07/schema#", schemaFromFile["$schema"])
	assert.Equal(t, schemaID, schemaFromFile["$id"])
	assert.Contains(t, schemaFromFile, "properties")

	props, ok := schemaFromFile["properties"].(map[string]any)
	require.True(t, ok)

	assert.Contains(t, props, "watchNamespace")
	assert.Contains(t, props, "deploymentConfig")

	deployConfig, ok := props["deploymentConfig"].(map[string]any)
	require.True(t, ok)

	dcProps, ok := deployConfig["properties"].(map[string]any)
	require.True(t, ok)

	expectedFields := []string{
		"nodeSelector", "tolerations", "resources", "env", "envFrom",
		"volumes", "volumeMounts", "affinity", "annotations",
	}

	for _, field := range expectedFields {
		assert.Contains(t, dcProps, field, "deploymentConfig should include %s", field)
	}

	assert.NotContains(t, dcProps, "selector", "selector field should be excluded")
}

func TestSchemaIsValidJSON(t *testing.T) {
	mockOpenAPI := getMockOpenAPISpec()
	fields := []fieldInfo{
		{JSONName: "tolerations", TypePkg: "corev1", TypeName: "Toleration", IsSlice: true},
	}

	s := generateBundleConfigSchema(mockOpenAPI, fields)

	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err, "should marshal schema to JSON")

	var unmarshaled map[string]any
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err, "generated JSON should be valid and unmarshalable")

	assert.Contains(t, unmarshaled, "$schema")
	assert.Contains(t, unmarshaled, "$id")
	assert.Contains(t, unmarshaled, "type")
	assert.Contains(t, unmarshaled, "properties")
}
