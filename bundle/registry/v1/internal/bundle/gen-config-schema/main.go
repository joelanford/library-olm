package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	schemaID          = "https://operator-framework.io/schemas/registry-v1-bundle-config.json"
	schemaDraft       = "http://json-schema.org/draft-07/schema#"
	schemaTitle       = "Registry+v1 Bundle Configuration"
	schemaDescription = "Configuration schema for registry+v1 bundles. Includes watchNamespace for controlling operator scope and deploymentConfig for customizing operator deployment (environment variables, resource scheduling, storage, and pod placement). The deploymentConfig follows the same structure and behavior as OLM v0's SubscriptionConfig. Note: The 'selector' field from v0's SubscriptionConfig is not included as it was never used."

	subscriptionConfigRelPath = "pkg/operators/v1alpha1/subscription_types.go"
)

// openAPISpec represents the structure of Kubernetes OpenAPI v3 spec
type openAPISpec struct {
	Components struct {
		Schemas map[string]any `json:"schemas"`
	} `json:"components"`
}

// schema represents a JSON Schema Draft 7 document with OpenAPI v3 components
type schema struct {
	Schema               string                  `json:"$schema"`
	ID                   string                  `json:"$id"`
	Title                string                  `json:"title"`
	Description          string                  `json:"description"`
	Type                 string                  `json:"type"`
	Properties           map[string]*schemaField `json:"properties"`
	AdditionalProperties bool                    `json:"additionalProperties"`
	Components           map[string]any          `json:"components,omitempty"`
}

// schemaField represents a single field in a JSON Schema
type schemaField struct {
	Type                 string                  `json:"type,omitempty"`
	Description          string                  `json:"description,omitempty"`
	Properties           map[string]*schemaField `json:"properties,omitempty"`
	AdditionalProperties any                     `json:"additionalProperties,omitempty"`
	Items                any                     `json:"items,omitempty"`
	AnyOf                []*schemaField          `json:"anyOf,omitempty"`
	AllOf                []*schemaField          `json:"allOf,omitempty"`
	Ref                  string                  `json:"$ref,omitempty"`

	// Allow pass-through of unknown fields from OpenAPI schemas
	Extra map[string]any `json:"-"`
}

// fieldInfo contains parsed information about a struct field
type fieldInfo struct {
	JSONName string
	TypeName string
	TypePkg  string
	IsSlice  bool
	IsPtr    bool
	IsMap    bool
}

// schemaCollector tracks schemas that need to be included for $ref resolution
type schemaCollector struct {
	openAPISpec      *openAPISpec
	collectedSchemas map[string]bool
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <output-file>\n", os.Args[0])
		os.Exit(1)
	}
	outputFile := os.Args[1]

	k8sVersion, err := resolveK8sVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving k8s.io/api version: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Resolved Kubernetes version: %s\n", k8sVersion)

	openAPISpecURL := fmt.Sprintf(
		"https://raw.githubusercontent.com/kubernetes/kubernetes/refs/tags/%s/api/openapi-spec/v3/api__v1_openapi.json",
		k8sVersion,
	)
	fmt.Printf("Fetching Kubernetes OpenAPI spec from %s...\n", openAPISpecURL)

	// Fetch the Kubernetes OpenAPI spec
	openAPISpec, err := fetchOpenAPISpec(openAPISpecURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	subscriptionTypesFile, err := resolveSubscriptionTypesFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving SubscriptionConfig source: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Parsing SubscriptionConfig from %s...\n", subscriptionTypesFile)

	// Parse SubscriptionConfig structure
	fields, err := parseSubscriptionConfig(subscriptionTypesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing SubscriptionConfig: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generating registry+v1 bundle configuration schema...\n")

	// Generate the schema
	schema := generateBundleConfigSchema(openAPISpec, fields)

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling schema: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	// Ensure output directory exists
	dir := filepath.Dir(outputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Write to file
	if err := os.WriteFile(outputFile, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing schema file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated schema at %s\n", outputFile)
}

func resolveK8sVersion() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Version}}", "k8s.io/api").Output()
	if err != nil {
		return "", fmt.Errorf("go list -m k8s.io/api: %w", err)
	}
	modVersion := strings.TrimSpace(string(out))
	if modVersion == "" {
		return "", fmt.Errorf("k8s.io/api version is empty")
	}
	k8sVersion := strings.Replace(modVersion, "v0.", "v1.", 1)
	return k8sVersion, nil
}

func resolveSubscriptionTypesFile() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/operator-framework/api").Output()
	if err != nil {
		return "", fmt.Errorf("go list -m github.com/operator-framework/api: %w", err)
	}
	moduleDir := strings.TrimSpace(string(out))
	if moduleDir == "" {
		return "", fmt.Errorf("operator-framework/api module dir is empty")
	}
	typesFile := filepath.Join(moduleDir, subscriptionConfigRelPath)
	if _, err := os.Stat(typesFile); err != nil {
		return "", fmt.Errorf("subscription types file not found: %w", err)
	}
	return typesFile, nil
}

func fetchOpenAPISpec(url string) (*openAPISpec, error) {
	// Create HTTP client with timeout to prevent hanging
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spec: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var spec openAPISpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return &spec, nil
}

func parseSubscriptionConfig(filePath string) ([]fieldInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var fields []fieldInfo

	// Find the SubscriptionConfig struct
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "SubscriptionConfig" {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Extract field information
		for _, field := range structType.Fields.List {
			if field.Names == nil {
				continue
			}

			fieldName := field.Names[0].Name

			// Skip Selector field
			if fieldName == "Selector" {
				continue
			}

			// Get JSON tag
			jsonName := extractJSONTag(field.Tag)
			if jsonName == "" || jsonName == "-" {
				continue
			}

			// Parse the field type
			fieldInfo := fieldInfo{
				JSONName: jsonName,
			}

			parseFieldType(field.Type, &fieldInfo)

			fields = append(fields, fieldInfo)
		}

		return false
	})

	return fields, nil
}

func extractJSONTag(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}

	tagValue := strings.Trim(tag.Value, "`")
	for _, part := range strings.Split(tagValue, " ") {
		if strings.HasPrefix(part, "json:") {
			jsonTag := strings.Trim(strings.TrimPrefix(part, "json:"), "\"")
			return strings.Split(jsonTag, ",")[0]
		}
	}

	return ""
}

func parseFieldType(expr ast.Expr, info *fieldInfo) {
	switch t := expr.(type) {
	case *ast.ArrayType:
		info.IsSlice = true
		parseFieldType(t.Elt, info)

	case *ast.StarExpr:
		info.IsPtr = true
		parseFieldType(t.X, info)

	case *ast.MapType:
		info.IsMap = true
		info.TypeName = "map[string]string" // Simplified for our use case

	case *ast.Ident:
		info.TypeName = t.Name

	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			info.TypePkg = pkg.Name
			info.TypeName = t.Sel.Name
		}
	}
}

func generateBundleConfigSchema(openAPISpec *openAPISpec, fields []fieldInfo) *schema {
	schema := &schema{
		Schema:               schemaDraft,
		ID:                   schemaID,
		Title:                schemaTitle,
		Description:          schemaDescription,
		Type:                 "object",
		Properties:           make(map[string]*schemaField),
		AdditionalProperties: false,
	}

	// Track schemas we need to include (for resolving $ref dependencies)
	collector := &schemaCollector{
		openAPISpec:      openAPISpec,
		collectedSchemas: make(map[string]bool),
	}

	// Add watchNamespace property (base definition - will be modified at runtime)
	schema.Properties["watchNamespace"] = &schemaField{
		Description: "The namespace that the operator should watch for custom resources. The meaning and validation of this field depends on the operator's install modes. This field may be optional or required, and may have format constraints, based on the operator's supported install modes.",
		AnyOf: []*schemaField{
			{Type: "null"},
			{Type: "string"},
		},
	}

	// Create deploymentConfig property
	deploymentConfigProps := make(map[string]*schemaField)

	// Build deploymentConfig properties from parsed fields
	for _, field := range fields {
		fieldSchema := mapFieldToOpenAPISchema(field, openAPISpec, collector)
		if fieldSchema != nil {
			deploymentConfigProps[field.JSONName] = fieldSchema
		}
	}

	schema.Properties["deploymentConfig"] = &schemaField{
		Type:                 "object",
		Description:          "Configuration for customizing operator deployment (environment variables, resources, volumes, etc.)",
		Properties:           deploymentConfigProps,
		AdditionalProperties: false,
	}

	// Add all collected schemas to the components/schemas section
	// (OpenAPI v3 uses components/schemas for $ref resolution)
	if len(collector.collectedSchemas) > 0 {
		componentsSchemas := make(map[string]any)
		for schemaName := range collector.collectedSchemas {
			if s, ok := openAPISpec.Components.Schemas[schemaName]; ok {
				componentsSchemas[schemaName] = s
			}
		}

		schema.Components = map[string]any{
			"schemas": componentsSchemas,
		}
	}

	return schema
}

func mapFieldToOpenAPISchema(field fieldInfo, openAPISpec *openAPISpec, collector *schemaCollector) *schemaField {
	// Handle map types (nodeSelector, annotations)
	if field.IsMap {
		return &schemaField{
			Type: "object",
			AdditionalProperties: &schemaField{
				Type: "string",
			},
		}
	}

	// Get the OpenAPI schema for the base type
	openAPITypeName := getOpenAPITypeName(field)
	if openAPITypeName == "" {
		fmt.Fprintf(os.Stderr, "Warning: Could not map field %s (type: %s.%s) to OpenAPI schema\n",
			field.JSONName, field.TypePkg, field.TypeName)
		return nil
	}

	baseSchema, ok := openAPISpec.Components.Schemas[openAPITypeName]
	if !ok {
		fmt.Fprintf(os.Stderr, "Warning: Schema for %s not found in OpenAPI spec\n", openAPITypeName)
		return nil
	}

	// Collect this schema and all its dependencies
	collector.collectSchemaWithDependencies(openAPITypeName, baseSchema)

	// Use $ref to point to the schema in components/schemas.
	// This preserves all validation keywords (required, enum, format, pattern, etc.)
	// that would be lost if we copied the schema content via marshal/unmarshal.
	schemaRef := &schemaField{
		Ref: fmt.Sprintf("#/components/schemas/%s", openAPITypeName),
	}

	// Wrap in array if it's a slice field
	if field.IsSlice {
		return &schemaField{
			Type:  "array",
			Items: schemaRef,
		}
	}

	return schemaRef
}

// collectSchemaWithDependencies recursively collects a schema and all schemas it references via $ref
func (c *schemaCollector) collectSchemaWithDependencies(schemaName string, schema any) {
	// Mark this schema as collected
	if c.collectedSchemas[schemaName] {
		return // Already processed
	}
	c.collectedSchemas[schemaName] = true

	// Recursively find all $ref references in this schema
	c.findReferences(schema)
}

// findReferences recursively walks a schema object to find all $ref pointers
func (c *schemaCollector) findReferences(obj any) {
	switch v := obj.(type) {
	case map[string]any:
		// Check if this is a $ref and process it
		c.processRef(v)

		// Recursively check all values in the map
		for _, val := range v {
			c.findReferences(val)
		}

	case []any:
		// Recursively check all items in the array
		for _, item := range v {
			c.findReferences(item)
		}
	}
}

// processRef extracts and collects schema dependencies from a $ref pointer
func (c *schemaCollector) processRef(v map[string]any) {
	ref, ok := v["$ref"].(string)
	if !ok {
		return
	}

	// Extract the schema name from the $ref
	// Format: "#/components/schemas/io.k8s.api.core.v1.NodeAffinity"
	if !strings.HasPrefix(ref, "#/components/schemas/") {
		return
	}

	schemaName := strings.TrimPrefix(ref, "#/components/schemas/")

	// Skip if already collected
	if c.collectedSchemas[schemaName] {
		return
	}

	// Collect the referenced schema recursively
	refSchema, ok := c.openAPISpec.Components.Schemas[schemaName]
	if ok {
		c.collectSchemaWithDependencies(schemaName, refSchema)
	}
}

func getOpenAPITypeName(field fieldInfo) string {
	// Map package names to OpenAPI prefixes
	pkgMap := map[string]string{
		"corev1": "io.k8s.api.core.v1",
		"v1":     "io.k8s.api.core.v1",
	}

	prefix, ok := pkgMap[field.TypePkg]
	if !ok {
		return ""
	}

	return fmt.Sprintf("%s.%s", prefix, field.TypeName)
}
