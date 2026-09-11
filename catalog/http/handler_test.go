package cataloghttp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	testutil "github.com/joelanford/library-olm/internal/util/test"
)

func TestNewHandlerConstructionAndMounting(t *testing.T) {
	t.Parallel()

	catalog := &testutil.Catalog{CatalogName: "catalog"}
	api := NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}})
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", api))

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/catalogs", nil))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	var collection catalogCollection
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &collection))
	require.Len(t, collection.Catalogs, 1)
	assert.Equal(t, "/api/v1/catalogs/catalog", collection.Catalogs[0].Links.Self.Href)

	followed := httptest.NewRecorder()
	mux.ServeHTTP(followed, httptest.NewRequest(http.MethodGet, collection.Catalogs[0].Links.Self.Href, nil))
	assert.Equal(t, http.StatusOK, followed.Code)
	var detail catalogDetail
	require.NoError(t, json.Unmarshal(followed.Body.Bytes(), &detail))
	assert.Equal(t, "/api/v1/catalogs/catalog", detail.Links.Self.Href)

	t.Run("absolute-form request target", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://untrusted.example/api/v1/catalogs", nil)
		request.RequestURI = "http://untrusted.example/api/v1/catalogs"
		request.URL.Path = "/v1/catalogs"
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		var got catalogCollection
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &got))
		require.Len(t, got.Catalogs, 1)
		assert.Equal(t, "/api/v1/catalogs/catalog", got.Catalogs[0].Links.Self.Href)
	})
}

func TestHandlerRecognizesEveryContractRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{method: http.MethodGet, path: "/v1/catalogs", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/packages", wantStatus: http.StatusOK},
		{method: http.MethodPost, path: "/v1/recommendations/cert-manager", body: `{}`, wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager/channels", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager/channels/stable:1", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager/bundles", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager/channels/stable:1/bundles", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager/bundles/cert-manager.v1.2.3", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/v1/catalogs/redhat/packages/cert-manager/icon", wantStatus: http.StatusOK},
	}

	bundle := testutil.NewBundle(t, "cert-manager", "1.2.3", "")
	leaf := &testutil.LeafGraph{GraphName: "1", Bundles: []bundlev1.Bundle{bundle}}
	stable := &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "stable", Bundles: []bundlev1.Bundle{bundle}},
		Graphs:    map[string]catalogv1.UpdateGraph{"1": leaf},
	}
	pkg := &testutil.CompositePackage{
		CompositeGraph: &testutil.CompositeGraph{
			LeafGraph: &testutil.LeafGraph{GraphName: "cert-manager", Bundles: []bundlev1.Bundle{bundle}},
			Graphs:    map[string]catalogv1.UpdateGraph{"stable": stable},
		},
		PackageIcon: catalogv1.Icon{Content: io.NopCloser(bytes.NewBufferString("icon")), MediaType: "image/png"},
	}
	api := NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{
		&testutil.Catalog{CatalogName: "redhat", Packages: map[string]catalogv1.Package{"cert-manager": pkg}},
	}})
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()

			api.ServeHTTP(response, request)

			assert.Equal(t, test.wantStatus, response.Code)
		})
	}
}

func TestHandlerUsesDecodedPathValues(t *testing.T) {
	t.Parallel()

	leaf := &testutil.LeafGraph{GraphName: "1/2"}
	stable := &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "stable channel"},
		Graphs:    map[string]catalogv1.UpdateGraph{"1/2": leaf},
	}
	pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "p"},
		Graphs:    map[string]catalogv1.UpdateGraph{"stable channel": stable, "stable": &testutil.LeafGraph{GraphName: "stable"}},
	}}
	reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
		&testutil.Catalog{CatalogName: "red hat/catalog"},
		&testutil.Catalog{CatalogName: "c", Packages: map[string]catalogv1.Package{"p": pkg}},
	}}
	api := NewHandler(reader)

	t.Run("encoded catalog name", func(t *testing.T) {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/catalogs/red%20hat%2Fcatalog", nil))

		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("colon-delimited channel path with encoded slash", func(t *testing.T) {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/catalogs/c/packages/p/channels/stable%20channel:1%2F2", nil))

		assert.Equal(t, http.StatusOK, response.Code)
		var detail channelDetail
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &detail))
		assert.Equal(t, []string{"stable channel", "1/2"}, detail.Path)
	})

	t.Run("encoded colon remains a path delimiter", func(t *testing.T) {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/catalogs/c/packages/p/channels/stable%3A1", nil))

		assert.Equal(t, http.StatusNotFound, response.Code)
		assertProblem(t, response, problemNotFound)
	})

	t.Run("encoded leading colon creates an empty segment", func(t *testing.T) {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/catalogs/c/packages/p/channels/%3Astable", nil))

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assertProblem(t, response, problemMalformedInput)
	})
}

func TestHandlerReturnsProblemDetailsForUnknownRoutesWithoutRedirects(t *testing.T) {
	t.Parallel()

	api := NewHandler(&testutil.StoreReader{})
	for _, path := range []string{
		"/",
		"/v1",
		"/v1/catalogs/",
		"/v1//catalogs",
		"//v1/catalogs",
		"/v1/catalogs//redhat",
		"/v1/unknown",
		"/v1/catalogs/redhat/packages",
		"/v1/catalogs/redhat/packages/cert-manager/unknown",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))

			assert.Equal(t, http.StatusNotFound, response.Code)
			assert.Empty(t, response.Header().Get("Location"))
			assertProblem(t, response, problemNotFound)
		})
	}
}

func TestHandlerReturnsMethodNotAllowedForKnownRoutes(t *testing.T) {
	t.Parallel()

	api := NewHandler(&testutil.StoreReader{})
	tests := []struct {
		method string
		path   string
		allow  string
	}{
		{method: http.MethodPatch, path: "/v1/catalogs", allow: http.MethodGet},
		{method: http.MethodPost, path: "/v1/catalogs/c", allow: http.MethodGet},
		{method: http.MethodHead, path: "/v1/packages", allow: http.MethodGet},
		{method: http.MethodGet, path: "/v1/recommendations/pkg", allow: http.MethodPost},
		{method: http.MethodPost, path: "/v1/catalogs/c/packages/p", allow: http.MethodGet},
		{method: http.MethodDelete, path: "/v1/catalogs/c/packages/p/channels", allow: http.MethodGet},
		{method: http.MethodPatch, path: "/v1/catalogs/c/packages/p/channels/stable", allow: http.MethodGet},
		{method: http.MethodPost, path: "/v1/catalogs/c/packages/p/bundles", allow: http.MethodGet},
		{method: http.MethodPatch, path: "/v1/catalogs/c/packages/p/channels/stable/bundles", allow: http.MethodGet},
		{method: http.MethodDelete, path: "/v1/catalogs/c/packages/p/bundles/p.v1.0.0", allow: http.MethodGet},
		{method: http.MethodPost, path: "/v1/catalogs/c/packages/p/icon", allow: http.MethodGet},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			api.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))

			assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
			assert.Equal(t, test.allow, response.Header().Get("Allow"))
			assertProblem(t, response, problemMethodNotAllowed)
		})
	}
}

func TestHandlerValidatesRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		kind        problemKind
	}{
		{name: "unknown query", method: http.MethodGet, path: "/v1/catalogs?sort=name", kind: problemMalformedInput},
		{name: "duplicate query", method: http.MethodGet, path: "/v1/packages?limit=1&limit=2", kind: problemMalformedInput},
		{name: "invalid selector", method: http.MethodGet, path: "/v1/catalogs?catalogSelector=bad!", kind: problemInvalidCatalogSelector},
		{name: "empty channel segment", method: http.MethodGet, path: "/v1/catalogs/c/packages/p/channels/a::b", kind: problemMalformedInput},
		{name: "unsupported body", method: http.MethodPost, path: "/v1/recommendations/p", body: `{}`, contentType: "text/plain", kind: problemUnsupportedMediaType},
		{name: "unknown body field", method: http.MethodPost, path: "/v1/recommendations/p", body: `{"unknown":true}`, contentType: "application/json", kind: problemMalformedInput},
	}

	api := NewHandler(&testutil.StoreReader{})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()

			api.ServeHTTP(response, request)

			assert.Equal(t, test.kind.status, response.Code)
			assertProblem(t, response, test.kind)
		})
	}
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, kind problemKind) problemDetails {
	t.Helper()

	assert.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
	var problem problemDetails
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &problem))
	assert.Equal(t, problemTypeBase+kind.typeName, problem.Type)
	assert.Equal(t, kind.title, problem.Title)
	assert.Equal(t, kind.status, problem.Status)
	assert.NotEmpty(t, problem.Instance)
	return problem
}
