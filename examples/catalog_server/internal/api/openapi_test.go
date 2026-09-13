package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
	testutil "github.com/joelanford/library-olm/examples/catalog_server/internal/testutil"
)

const openAPIPath = "openapi.yaml"

func TestOpenAPIStructuralCompleteness(t *testing.T) {
	doc := loadOpenAPI(t)

	assert.Equal(t, "3.1.0", doc["openapi"])

	paths := objectAt(t, doc, "paths")
	expected := map[string]string{
		"/catalogs":                              "get",
		"/catalogs/{catalog}":                    "get",
		"/packages":                              "get",
		"/recommendations/{package}":             "post",
		"/catalogs/{catalog}/packages/{package}": "get",
		"/catalogs/{catalog}/packages/{package}/channels":                       "get",
		"/catalogs/{catalog}/packages/{package}/channels/{channelPath}":         "get",
		"/catalogs/{catalog}/packages/{package}/bundles":                        "get",
		"/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles": "get",
		"/catalogs/{catalog}/packages/{package}/bundles/{bundleID}":             "get",
		"/catalogs/{catalog}/packages/{package}/icon":                           "get",
	}
	require.Len(t, paths, len(expected))

	for path, method := range expected {
		pathItem := objectAt(t, paths, path)
		operation := objectAt(t, pathItem, method)
		assert.NotEmpty(t, operation["operationId"], "%s %s has no operationId", method, path)

		responses := objectAt(t, operation, "responses")
		assert.Contains(t, responses, "200", "%s %s has no success response", method, path)
		assert.Contains(t, responses, "405", "%s %s has no method response", method, path)
		assert.Contains(t, responses, "500", "%s %s has no internal failure response", method, path)
	}
	for _, path := range []string{
		"/catalogs/{catalog}",
		"/catalogs/{catalog}/packages/{package}",
		"/catalogs/{catalog}/packages/{package}/bundles/{bundleID}",
		"/catalogs/{catalog}/packages/{package}/icon",
	} {
		responses := objectAt(t, objectAt(t, objectAt(t, paths, path), "get"), "responses")
		assert.Contains(t, responses, "400", "get %s does not declare malformed input", path)
	}

	assertSelectorUsage(t, paths)
	assertChannelPathContract(t, doc)
	assertCollectionContract(t, doc)
	assertIconContract(t, paths)
	assertProblemContract(t, doc)
}

func TestOpenAPIComponentSchemasCompile(t *testing.T) {
	doc := loadOpenAPI(t)
	components := objectAt(t, objectAt(t, doc, "components"), "schemas")

	for _, name := range []string{
		"CatalogCollection",
		"PackageDetail",
		"ChannelCollection",
		"BundleDetail",
		"RecommendationRequest",
		"BundleSummary",
		"Recommendation",
		"MalformedInputProblem",
		"StaleCursorProblem",
	} {
		t.Run(name, func(t *testing.T) {
			root := map[string]any{
				"$schema":    "https://json-schema.org/draft/2020-12/schema",
				"components": map[string]any{"schemas": components},
				"$ref":       "#/components/schemas/" + name,
			}

			compiler := jsonschema.NewCompiler()
			require.NoError(t, compiler.AddResource("schema.json", root))
			compiled, err := compiler.Compile("schema.json")
			require.NoError(t, err)
			require.NotNil(t, compiled)
		})
	}
}

func TestBundleSummaryOpenAPIContract(t *testing.T) {
	doc := loadOpenAPI(t)
	schemas := objectAt(t, objectAt(t, doc, "components"), "schemas")
	summary := objectAt(t, schemas, "BundleSummary")
	assert.Contains(t, summary["required"], "uri")
	assert.Equal(t, "string", objectAt(t, objectAt(t, summary, "properties"), "uri")["type"])
	assert.NotContains(t, schemas, "RecommendationCandidate")

	bundleCollectionSchema := objectAt(t, schemas, "BundleCollection")
	bundles := objectAt(t, objectAt(t, bundleCollectionSchema, "properties"), "bundles")
	assert.Equal(t, "#/components/schemas/BundleSummary", objectAt(t, bundles, "items")["$ref"])
	recommendationSchema := objectAt(t, schemas, "Recommendation")
	candidates := objectAt(t, objectAt(t, recommendationSchema, "properties"), "candidates")
	assert.Equal(t, "#/components/schemas/BundleSummary", objectAt(t, candidates, "items")["$ref"])

	for _, test := range []struct {
		name        string
		method      string
		requestPath string
		openAPIPath string
		field       string
	}{
		{name: "package bundle collection", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/bundles", openAPIPath: "/catalogs/{catalog}/packages/{package}/bundles", field: "bundles"},
		{name: "channel bundle collection", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/channels/stable/bundles", openAPIPath: "/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles", field: "bundles"},
		{name: "recommendation", method: http.MethodPost, requestPath: "/v1/recommendations/package", openAPIPath: "/recommendations/{package}", field: "candidates"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var body io.Reader
			if test.method == http.MethodPost {
				body = strings.NewReader(`{}`)
			}
			request := httptest.NewRequest(test.method, test.requestPath, body)
			if body != nil {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			contractHandler(t).ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			responseSchema := operationResponseSchema(t, doc, test.openAPIPath, strings.ToLower(test.method), response.Code, "application/json")
			validateJSON(t, doc, responseSchema, response.Body.Bytes())

			var result map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			items, ok := result[test.field].([]any)
			require.True(t, ok)
			require.Len(t, items, 1)
			item, ok := items[0].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "oci://example/package:1.2.3", item["uri"])
		})
	}
}

func TestHandlerSuccessesMatchOpenAPI(t *testing.T) {
	doc := loadOpenAPI(t)
	handler := contractHandler(t)
	tests := []struct {
		name        string
		method      string
		requestPath string
		openAPIPath string
		body        string
		contentType string
	}{
		{name: "catalog collection", method: http.MethodGet, requestPath: "/v1/catalogs", openAPIPath: "/catalogs", contentType: "application/json"},
		{name: "catalog detail", method: http.MethodGet, requestPath: "/v1/catalogs/catalog", openAPIPath: "/catalogs/{catalog}", contentType: "application/json"},
		{name: "package collection", method: http.MethodGet, requestPath: "/v1/packages", openAPIPath: "/packages", contentType: "application/json"},
		{name: "recommendation", method: http.MethodPost, requestPath: "/v1/recommendations/package", openAPIPath: "/recommendations/{package}", body: `{"channelPaths":[["stable"]],"upgradeConstraintPolicy":"SelfCertified"}`, contentType: "application/json"},
		{name: "package detail", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package", openAPIPath: "/catalogs/{catalog}/packages/{package}", contentType: "application/json"},
		{name: "channel collection", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/channels", openAPIPath: "/catalogs/{catalog}/packages/{package}/channels", contentType: "application/json"},
		{name: "channel detail", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/channels/stable", openAPIPath: "/catalogs/{catalog}/packages/{package}/channels/{channelPath}", contentType: "application/json"},
		{name: "package bundle collection", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/bundles", openAPIPath: "/catalogs/{catalog}/packages/{package}/bundles", contentType: "application/json"},
		{name: "channel bundle collection", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/channels/stable/bundles", openAPIPath: "/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles", contentType: "application/json"},
		{name: "bundle detail", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/bundles/package.v1.2.3", openAPIPath: "/catalogs/{catalog}/packages/{package}/bundles/{bundleID}", contentType: "application/json"},
		{name: "icon", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/icon", openAPIPath: "/catalogs/{catalog}/packages/{package}/icon", contentType: "image/png"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body io.Reader
			if test.body != "" {
				requestSchema := operationRequestSchema(t, doc, test.openAPIPath, strings.ToLower(test.method), "application/json")
				validateJSON(t, doc, requestSchema, []byte(test.body))
				body = strings.NewReader(test.body)
			}
			request := httptest.NewRequest(test.method, test.requestPath, body)
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, test.contentType, response.Header().Get("Content-Type"))
			responseSchema := operationResponseSchema(t, doc, test.openAPIPath, strings.ToLower(test.method), response.Code, test.contentType)
			if test.contentType == "application/json" {
				validateJSON(t, doc, responseSchema, response.Body.Bytes())
			} else {
				assert.Equal(t, "icon bytes", response.Body.String())
				assert.Equal(t, "string", responseSchema["type"])
				assert.Equal(t, "binary", responseSchema["format"])
				assert.Equal(t, "attachment", response.Header().Get("Content-Disposition"))
				assert.Equal(t, "default-src 'none'; sandbox", response.Header().Get("Content-Security-Policy"))
				assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
			}
		})
	}
}

func TestHandlerProblemsMatchOpenAPI(t *testing.T) {
	doc := loadOpenAPI(t)
	tests := []struct {
		name        string
		method      string
		requestPath string
		openAPIPath string
		body        string
		contentType string
		status      int
		typeName    string
		reader      *testutil.StoreReader
	}{
		{name: "malformed input", method: http.MethodGet, requestPath: "/v1/catalogs?limit=0", openAPIPath: "/catalogs", status: 400, typeName: "malformed-input"},
		{name: "catalog detail malformed input", method: http.MethodGet, requestPath: "/v1/catalogs/catalog?unknown=true", openAPIPath: "/catalogs/{catalog}", status: 400, typeName: "malformed-input"},
		{name: "package detail malformed input", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package?unknown=true", openAPIPath: "/catalogs/{catalog}/packages/{package}", status: 400, typeName: "malformed-input"},
		{name: "bundle detail malformed input", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/bundles/package.v1.2.3?unknown=true", openAPIPath: "/catalogs/{catalog}/packages/{package}/bundles/{bundleID}", status: 400, typeName: "malformed-input"},
		{name: "icon malformed input", method: http.MethodGet, requestPath: "/v1/catalogs/catalog/packages/package/icon?unknown=true", openAPIPath: "/catalogs/{catalog}/packages/{package}/icon", status: 400, typeName: "malformed-input"},
		{name: "invalid selector", method: http.MethodGet, requestPath: "/v1/catalogs?catalogSelector=bad!", openAPIPath: "/catalogs", status: 400, typeName: "invalid-catalog-selector"},
		{name: "invalid constraint", method: http.MethodPost, requestPath: "/v1/recommendations/package", openAPIPath: "/recommendations/{package}", body: `{"versionConstraint":"nope"}`, contentType: "application/json", status: 400, typeName: "invalid-version-constraint"},
		{name: "unsupported policy", method: http.MethodPost, requestPath: "/v1/recommendations/package", openAPIPath: "/recommendations/{package}", body: `{"upgradeConstraintPolicy":"Other"}`, contentType: "application/json", status: 400, typeName: "unsupported-upgrade-constraint-policy"},
		{name: "invalid cursor", method: http.MethodGet, requestPath: "/v1/catalogs?cursor=bad", openAPIPath: "/catalogs", status: 400, typeName: "invalid-cursor"},
		{name: "not found", method: http.MethodGet, requestPath: "/v1/catalogs/missing", openAPIPath: "/catalogs/{catalog}", status: 404, typeName: "not-found"},
		{name: "method not allowed", method: http.MethodPost, requestPath: "/v1/catalogs", openAPIPath: "/catalogs", status: 405, typeName: "method-not-allowed"},
		{name: "stale cursor", method: http.MethodGet, requestPath: "/v1/catalogs?cursor=" + contractStaleCursor(t), openAPIPath: "/catalogs", status: 409, typeName: "stale-cursor"},
		{name: "ambiguous package", method: http.MethodPost, requestPath: "/v1/recommendations/package", openAPIPath: "/recommendations/{package}", body: `{}`, contentType: "application/json", status: 409, typeName: "ambiguous-package", reader: ambiguousContractReader()},
		{name: "unsupported media type", method: http.MethodPost, requestPath: "/v1/recommendations/package", openAPIPath: "/recommendations/{package}", body: `{}`, contentType: "text/plain", status: 415, typeName: "unsupported-media-type"},
		{name: "catalog failure", method: http.MethodGet, requestPath: "/v1/catalogs", openAPIPath: "/catalogs", status: 500, typeName: "catalog-read-failure", reader: &testutil.StoreReader{ListErr: fmt.Errorf("failed")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := test.reader
			if reader == nil {
				reader = &testutil.StoreReader{}
			}
			request := httptest.NewRequest(test.method, test.requestPath, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			NewHandler(reader).ServeHTTP(response, request)

			assert.Equal(t, test.status, response.Code)
			assert.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
			operationMethod := strings.ToLower(test.method)
			if response.Code == http.StatusMethodNotAllowed {
				operationMethod = "get"
			}
			responseSchema := operationResponseSchema(t, doc, test.openAPIPath, operationMethod, response.Code, "application/problem+json")
			validateJSON(t, doc, responseSchema, response.Body.Bytes())
			var problem problemDetails
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &problem))
			assert.Equal(t, problemTypeBase+test.typeName, problem.Type)
			if response.Code == http.StatusMethodNotAllowed {
				assert.Equal(t, http.MethodGet, response.Header().Get("Allow"))
			}
		})
	}
}

func contractHandler(t *testing.T) http.Handler {
	t.Helper()
	timestamp := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	bundle := testutil.NewBundle(t, "package", "1.2.3", "")
	bundle.BundleURI = "oci://example/package:1.2.3"
	bundle.BundleMetadata = model.BundleMetadata{MediaType: "registry+v1", ReleaseTimestamp: &timestamp}
	channel := &testutil.LeafGraph{GraphName: "stable", Bundles: []bundlev1.Bundle{bundle}}
	pkg := &testutil.CompositePackage{
		CompositeGraph: &testutil.CompositeGraph{
			LeafGraph: &testutil.LeafGraph{GraphName: "package", Bundles: []bundlev1.Bundle{bundle}},
			Graphs:    map[string]catalogv1.UpdateGraph{"stable": channel},
		},
		PackageMetadata: model.PackageMetadata{DisplayName: "Package", Provider: model.Provider{Name: "Provider"}, IconAvailable: true},
		PackageIcon:     model.Icon{Content: []byte("icon bytes"), MediaType: "image/png"},
	}
	catalog := &testutil.Catalog{
		CatalogName:     "catalog",
		CatalogURI:      "oci://example/catalog",
		CatalogDigest:   "sha256:1234",
		CatalogPriority: 10,
		CatalogLabels:   map[string]string{"env": "test"},
		Packages:        map[string]catalogv1.UpdateGraph{"package": pkg},
	}
	return NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}})
}

func ambiguousContractReader() *testutil.StoreReader {
	packageA := &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "package"}}
	packageB := &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "package"}}
	return &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
		&testutil.Catalog{CatalogName: "a", Packages: map[string]catalogv1.UpdateGraph{"package": packageA}},
		&testutil.Catalog{CatalogName: "b", Packages: map[string]catalogv1.UpdateGraph{"package": packageB}},
	}}
}

func contractStaleCursor(t *testing.T) string {
	t.Helper()
	token, err := encodeCursor(catalogCursorKey{Name: "a"}, collectionRequest{}.normalizedInput("/v1/catalogs"), []catalogv1.Catalog{
		&testutil.Catalog{CatalogName: "a", CatalogDigest: "old"},
		&testutil.Catalog{CatalogName: "b"},
	})
	require.NoError(t, err)
	return token
}

func operationResponseSchema(t *testing.T, doc map[string]any, path, method string, status int, contentType string) map[string]any {
	t.Helper()
	operation := objectAt(t, objectAt(t, objectAt(t, doc, "paths"), path), method)
	responses := objectAt(t, operation, "responses")
	response := resolveReference(t, doc, objectAt(t, responses, fmt.Sprint(status)))
	content := objectAt(t, response, "content")
	media, ok := content[contentType]
	if !ok && contentType != "application/json" && contentType != "application/problem+json" {
		media = content["*/*"]
	}
	require.NotNil(t, media, "%s %s response %d does not declare %s", method, path, status, contentType)
	return objectAt(t, media.(map[string]any), "schema")
}

func operationRequestSchema(t *testing.T, doc map[string]any, path, method, contentType string) map[string]any {
	t.Helper()
	operation := objectAt(t, objectAt(t, objectAt(t, doc, "paths"), path), method)
	requestBody := objectAt(t, operation, "requestBody")
	content := objectAt(t, requestBody, "content")
	media := objectAt(t, content, contentType)
	return objectAt(t, media, "schema")
}

func validateJSON(t *testing.T, doc map[string]any, schema map[string]any, data []byte) {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal(data, &value))
	root := map[string]any{
		"$schema":    "https://json-schema.org/draft/2020-12/schema",
		"components": objectAt(t, doc, "components"),
	}
	for key, value := range schema {
		root[key] = value
	}
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", root))
	compiled, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(value))
}

func resolveReference(t *testing.T, doc map[string]any, value map[string]any) map[string]any {
	t.Helper()
	reference, ok := value["$ref"].(string)
	if !ok {
		return value
	}
	current := any(doc)
	for _, segment := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
		object, ok := current.(map[string]any)
		require.True(t, ok, "%q does not resolve to an object", reference)
		current, ok = object[segment]
		require.True(t, ok, "cannot resolve %q", reference)
	}
	result, ok := current.(map[string]any)
	require.True(t, ok, "%q does not resolve to an object", reference)
	return result
}

func loadOpenAPI(t *testing.T) map[string]any {
	t.Helper()

	yamlData, err := os.ReadFile(openAPIPath)
	require.NoError(t, err)
	jsonData, err := yaml.YAMLToJSON(yamlData)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(jsonData, &doc))
	return doc
}

func objectAt(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := object[key]
	require.True(t, ok, "missing object %q", key)
	result, ok := value.(map[string]any)
	require.True(t, ok, "%q is %T, not an object", key, value)
	return result
}

func arrayAt(t *testing.T, object map[string]any, key string) []any {
	t.Helper()

	value, ok := object[key]
	require.True(t, ok, "missing array %q", key)
	result, ok := value.([]any)
	require.True(t, ok, "%q is %T, not an array", key, value)
	return result
}

func assertSelectorUsage(t *testing.T, paths map[string]any) {
	t.Helper()

	var selectorOperations []string
	for path, pathValue := range paths {
		pathItem := pathValue.(map[string]any)
		for _, method := range []string{"get", "post"} {
			operationValue, ok := pathItem[method]
			if !ok {
				continue
			}
			operation := operationValue.(map[string]any)
			for _, parameterValue := range arrayOrEmpty(operation["parameters"]) {
				parameter := parameterValue.(map[string]any)
				if parameter["$ref"] == "#/components/parameters/CatalogSelector" {
					selectorOperations = append(selectorOperations, method+" "+path)
				}
			}
		}
	}

	assert.ElementsMatch(t, []string{
		"get /catalogs",
		"get /packages",
		"post /recommendations/{package}",
	}, selectorOperations)
}

func assertChannelPathContract(t *testing.T, doc map[string]any) {
	t.Helper()

	components := objectAt(t, doc, "components")
	parameters := objectAt(t, components, "parameters")
	pathParameter := objectAt(t, parameters, "ChannelPath")
	assert.Equal(t, "channelPath", pathParameter["name"])
	assert.Equal(t, "path", pathParameter["in"])
	assert.Equal(t, true, pathParameter["required"])
	assert.Equal(t, "^[^:]+(?::[^:]+)*$", objectAt(t, pathParameter, "schema")["pattern"])

	schemas := objectAt(t, components, "schemas")
	jsonPath := objectAt(t, schemas, "ChannelPath")
	assert.Equal(t, "array", jsonPath["type"])
	assert.EqualValues(t, 1, jsonPath["minItems"])
	segment := objectAt(t, schemas, "ChannelSegment")
	assert.Equal(t, "^[^:]+$", segment["pattern"])
}

func assertCollectionContract(t *testing.T, doc map[string]any) {
	t.Helper()

	components := objectAt(t, doc, "components")
	parameters := objectAt(t, components, "parameters")
	limit := objectAt(t, objectAt(t, parameters, "Limit"), "schema")
	assert.EqualValues(t, 50, limit["default"])
	assert.EqualValues(t, 200, limit["maximum"])

	schemas := objectAt(t, components, "schemas")
	for schemaName, itemField := range map[string]string{
		"CatalogCollection": "catalogs",
		"PackageCollection": "packages",
		"ChannelCollection": "channels",
		"BundleCollection":  "bundles",
		"Recommendation":    "candidates",
	} {
		schema := objectAt(t, schemas, schemaName)
		properties := objectAt(t, schema, "properties")
		assert.Contains(t, properties, itemField)
		assert.Contains(t, properties, "nextCursor")
		total := objectAt(t, properties, "total")
		assert.Equal(t, "integer", total["type"])
		assert.EqualValues(t, 0, total["minimum"])
		assert.Contains(t, schema["required"], "total")
	}
}

func assertIconContract(t *testing.T, paths map[string]any) {
	t.Helper()

	icon := objectAt(t, objectAt(t, paths, "/catalogs/{catalog}/packages/{package}/icon"), "get")
	responses := objectAt(t, icon, "responses")
	for status := range responses {
		assert.NotRegexp(t, `^3`, status, "icon redirects are deferred")
	}
	success := objectAt(t, responses, "200")
	headers := objectAt(t, success, "headers")
	for name, value := range map[string]string{
		"Content-Disposition":     "attachment",
		"Content-Security-Policy": "default-src 'none'; sandbox",
		"X-Content-Type-Options":  "nosniff",
	} {
		assert.Equal(t, value, objectAt(t, objectAt(t, headers, name), "schema")["const"])
	}
	content := objectAt(t, success, "content")
	binary := objectAt(t, objectAt(t, content, "*/*"), "schema")
	assert.Equal(t, "string", binary["type"])
	assert.Equal(t, "binary", binary["format"])
}

func assertProblemContract(t *testing.T, doc map[string]any) {
	t.Helper()

	schemas := objectAt(t, objectAt(t, doc, "components"), "schemas")
	expectedStatuses := map[string]float64{
		"MalformedInputProblem":           400,
		"InvalidCatalogSelectorProblem":   400,
		"InvalidVersionConstraintProblem": 400,
		"UnsupportedPolicyProblem":        400,
		"InvalidCursorProblem":            400,
		"NotFoundProblem":                 404,
		"MethodNotAllowedProblem":         405,
		"StaleCursorProblem":              409,
		"AmbiguousPackageProblem":         409,
		"UnsupportedMediaTypeProblem":     415,
		"CatalogReadFailureProblem":       500,
	}
	for name, status := range expectedStatuses {
		parts := arrayAt(t, objectAt(t, schemas, name), "allOf")
		specialization := parts[1].(map[string]any)
		properties := objectAt(t, specialization, "properties")
		assert.Equal(t, status, objectAt(t, properties, "status")["const"], name)
		problemType := objectAt(t, properties, "type")["const"]
		assert.Regexp(t, `^https://library-olm\.dev/problems/v1/`, problemType, name)
	}
}

func arrayOrEmpty(value any) []any {
	if value == nil {
		return nil
	}
	return value.([]any)
}
