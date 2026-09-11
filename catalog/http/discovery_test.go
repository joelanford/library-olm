package cataloghttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	testutil "github.com/joelanford/library-olm/internal/util/test"
)

func TestFakeStoreReaderSelectorNarrowsExistingSelection(t *testing.T) {
	t.Parallel()

	reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
		&testutil.Catalog{CatalogName: "prod-us", CatalogLabels: map[string]string{"env": "prod", "region": "us"}},
		&testutil.Catalog{CatalogName: "prod-eu", CatalogLabels: map[string]string{"env": "prod", "region": "eu"}},
		&testutil.Catalog{CatalogName: "dev-us", CatalogLabels: map[string]string{"env": "dev", "region": "us"}},
	}}
	environment, err := labels.Parse("env=prod")
	require.NoError(t, err)
	region, err := labels.Parse("region=us")
	require.NoError(t, err)

	selected := reader.Select(environment).Select(region)
	catalogs, err := selected.List()
	require.NoError(t, err)
	require.Len(t, catalogs, 1)
	assert.Equal(t, "prod-us", catalogs[0].Name())
}

func TestListCatalogsSelectorOrderingAndPagination(t *testing.T) {
	t.Parallel()

	root := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
		&testutil.Catalog{CatalogName: "prod-z", CatalogDigest: "z", CatalogLabels: map[string]string{"env": "prod", "region": "us"}},
		&testutil.Catalog{CatalogName: "dev-a", CatalogDigest: "dev", CatalogLabels: map[string]string{"env": "dev", "region": "us"}},
		&testutil.Catalog{CatalogName: "prod-a", CatalogDigest: "a", CatalogLabels: map[string]string{"env": "prod", "region": "us"}},
		&testutil.Catalog{CatalogName: "prod-eu", CatalogDigest: "eu", CatalogLabels: map[string]string{"env": "prod", "region": "eu"}},
	}}
	environment, err := labels.Parse("env=prod")
	require.NoError(t, err)
	base := root.Select(environment).(*testutil.StoreReader)
	api := NewHandler(base)

	firstResponse := httptest.NewRecorder()
	api.ServeHTTP(firstResponse, httptest.NewRequest(http.MethodGet, "/v1/catalogs?catalogSelector=region%3Dus&limit=1", nil))

	assert.Equal(t, http.StatusOK, firstResponse.Code)
	assert.Equal(t, "application/json", firstResponse.Header().Get("Content-Type"))
	var first catalogCollection
	require.NoError(t, json.Unmarshal(firstResponse.Body.Bytes(), &first))
	require.Len(t, first.Catalogs, 1)
	assert.Equal(t, "prod-a", first.Catalogs[0].Name)
	assert.Equal(t, 2, first.Total)
	assert.NotEmpty(t, first.NextCursor)
	assert.Equal(t, []string{"region=us"}, base.SelectCalls)
	require.NotNil(t, base.LastSelection)
	assert.Equal(t, 1, base.LastSelection.ListCalls)
	assert.Equal(t, 0, base.ListCalls)

	secondResponse := httptest.NewRecorder()
	path := "/v1/catalogs?catalogSelector=region%3Dus&limit=200&cursor=" + first.NextCursor
	api.ServeHTTP(secondResponse, httptest.NewRequest(http.MethodGet, path, nil))

	assert.Equal(t, http.StatusOK, secondResponse.Code)
	var second catalogCollection
	require.NoError(t, json.Unmarshal(secondResponse.Body.Bytes(), &second))
	require.Len(t, second.Catalogs, 1)
	assert.Equal(t, "prod-z", second.Catalogs[0].Name)
	assert.Equal(t, 2, second.Total)
	assert.Empty(t, second.NextCursor)
	assert.Equal(t, 1, base.LastSelection.ListCalls)
}

func TestListCatalogsLimitsAndEmptyArray(t *testing.T) {
	t.Parallel()

	catalogs := make([]catalogv1.Catalog, 201)
	for i := range catalogs {
		catalogs[i] = &testutil.Catalog{CatalogName: fmt.Sprintf("catalog-%03d", i)}
	}
	api := NewHandler(&testutil.StoreReader{Catalogs: catalogs})

	for _, test := range []struct {
		name      string
		path      string
		wantItems int
	}{
		{name: "default", path: "/v1/catalogs", wantItems: defaultLimit},
		{name: "maximum", path: "/v1/catalogs?limit=200", wantItems: maximumLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))

			assert.Equal(t, http.StatusOK, response.Code)
			var collection catalogCollection
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &collection))
			assert.Len(t, collection.Catalogs, test.wantItems)
			assert.NotEmpty(t, collection.NextCursor)
		})
	}

	emptyResponse := httptest.NewRecorder()
	NewHandler(&testutil.StoreReader{}).ServeHTTP(emptyResponse, httptest.NewRequest(http.MethodGet, "/v1/catalogs", nil))
	assert.Equal(t, http.StatusOK, emptyResponse.Code)
	assert.Equal(t, "application/json", emptyResponse.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"catalogs":[],"total":0}`, emptyResponse.Body.String())

	emptyResponse = httptest.NewRecorder()
	NewHandler(&testutil.StoreReader{}).ServeHTTP(emptyResponse, httptest.NewRequest(http.MethodGet, "/v1/packages", nil))
	assert.Equal(t, http.StatusOK, emptyResponse.Code)
	assert.Equal(t, "application/json", emptyResponse.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"packages":[],"total":0}`, emptyResponse.Body.String())
}

func TestListCatalogsCursorValidationAndBinding(t *testing.T) {
	t.Parallel()

	t.Run("stable snapshot continues", func(t *testing.T) {
		catalogs := []catalogv1.Catalog{
			&testutil.Catalog{CatalogName: "a", CatalogDigest: "1"},
			&testutil.Catalog{CatalogName: "b", CatalogDigest: "2"},
		}
		api := NewHandler(&testutil.StoreReader{Catalogs: catalogs})
		first := httptest.NewRecorder()
		api.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/catalogs?limit=1", nil))
		var collection catalogCollection
		require.NoError(t, json.Unmarshal(first.Body.Bytes(), &collection))

		continued := httptest.NewRecorder()
		api.ServeHTTP(continued, httptest.NewRequest(http.MethodGet, "/v1/catalogs?cursor="+collection.NextCursor, nil))
		assert.Equal(t, http.StatusOK, continued.Code)
	})

	t.Run("changed snapshot is stale", func(t *testing.T) {
		firstCatalog := &testutil.Catalog{CatalogName: "a", CatalogDigest: "1"}
		reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
			firstCatalog,
			&testutil.Catalog{CatalogName: "b", CatalogDigest: "2"},
		}}
		api := NewHandler(reader)
		first := httptest.NewRecorder()
		api.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/catalogs?limit=1", nil))
		var collection catalogCollection
		require.NoError(t, json.Unmarshal(first.Body.Bytes(), &collection))
		firstCatalog.CatalogDigest = "changed"

		continued := httptest.NewRecorder()
		api.ServeHTTP(continued, httptest.NewRequest(http.MethodGet, "/v1/catalogs?cursor="+collection.NextCursor, nil))
		assert.Equal(t, http.StatusConflict, continued.Code)
		assertProblem(t, continued, problemStaleCursor)
	})

	t.Run("removed catalog is stale", func(t *testing.T) {
		reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
			&testutil.Catalog{CatalogName: "a", CatalogDigest: "1"},
			&testutil.Catalog{CatalogName: "b", CatalogDigest: "2"},
		}}
		api := NewHandler(reader)
		first := httptest.NewRecorder()
		api.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/catalogs?limit=1", nil))
		var collection catalogCollection
		require.NoError(t, json.Unmarshal(first.Body.Bytes(), &collection))
		reader.Catalogs = reader.Catalogs[1:]

		continued := httptest.NewRecorder()
		api.ServeHTTP(continued, httptest.NewRequest(http.MethodGet, "/v1/catalogs?cursor="+collection.NextCursor, nil))
		assert.Equal(t, http.StatusConflict, continued.Code)
		assertProblem(t, continued, problemStaleCursor)
	})

	t.Run("cursor is endpoint specific", func(t *testing.T) {
		catalogs := []catalogv1.Catalog{
			&testutil.Catalog{CatalogName: "a"},
			&testutil.Catalog{CatalogName: "b"},
		}
		api := NewHandler(&testutil.StoreReader{Catalogs: catalogs})
		first := httptest.NewRecorder()
		api.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/v1/catalogs?limit=1", nil))
		var collection catalogCollection
		require.NoError(t, json.Unmarshal(first.Body.Bytes(), &collection))

		wrongEndpoint := httptest.NewRecorder()
		api.ServeHTTP(wrongEndpoint, httptest.NewRequest(http.MethodGet, "/v1/packages?cursor="+collection.NextCursor, nil))
		assert.Equal(t, http.StatusConflict, wrongEndpoint.Code)
		assertProblem(t, wrongEndpoint, problemStaleCursor)
	})

	t.Run("malformed and missing keys are invalid without panicking", func(t *testing.T) {
		catalogs := []catalogv1.Catalog{&testutil.Catalog{CatalogName: "a"}}
		api := NewHandler(&testutil.StoreReader{Catalogs: catalogs})
		missingKey, err := encodeCursor(catalogCursorKey{Name: "past-end"}, collectionRequest{}.normalizedInput("/v1/catalogs"), catalogs)
		require.NoError(t, err)

		for _, token := range []string{"not-a-cursor", missingKey} {
			response := httptest.NewRecorder()
			api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/catalogs?cursor="+token, nil))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assertProblem(t, response, problemInvalidCursor)
		}
	})

	t.Run("cursor after final item returns an empty page", func(t *testing.T) {
		catalogs := []catalogv1.Catalog{&testutil.Catalog{CatalogName: "a"}}
		token, err := encodeCursor(catalogCursorKey{Name: "a"}, collectionRequest{}.normalizedInput("/v1/catalogs"), catalogs)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		NewHandler(&testutil.StoreReader{Catalogs: catalogs}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/catalogs?cursor="+token, nil))

		assert.Equal(t, http.StatusOK, response.Code)
		assert.JSONEq(t, `{"catalogs":[],"total":1}`, response.Body.String())
	})
}

func TestGetCatalogOutcomes(t *testing.T) {
	t.Parallel()

	catalog := &testutil.Catalog{
		CatalogName:     "catalog",
		CatalogURI:      "oci://catalog",
		CatalogDigest:   "sha256:1",
		CatalogPriority: 7,
		CatalogLabels:   map[string]string{"env": "prod"},
	}
	success := httptest.NewRecorder()
	NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}).ServeHTTP(success, httptest.NewRequest(http.MethodGet, "/v1/catalogs/catalog", nil))
	assert.Equal(t, http.StatusOK, success.Code)
	assert.Equal(t, "application/json", success.Header().Get("Content-Type"))
	var detail catalogDetail
	require.NoError(t, json.Unmarshal(success.Body.Bytes(), &detail))
	assert.Equal(t, "catalog", detail.Name)

	notFound := httptest.NewRecorder()
	NewHandler(&testutil.StoreReader{}).ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/v1/catalogs/missing", nil))
	assert.Equal(t, http.StatusNotFound, notFound.Code)
	assertProblem(t, notFound, problemNotFound)

	internal := httptest.NewRecorder()
	NewHandler(&testutil.StoreReader{GetErr: errors.New("secret storage failure")}).ServeHTTP(internal, httptest.NewRequest(http.MethodGet, "/v1/catalogs/catalog", nil))
	assert.Equal(t, http.StatusInternalServerError, internal.Code)
	assertProblem(t, internal, problemCatalogReadFailure)
	assert.NotContains(t, internal.Body.String(), "secret")
}

func TestListPackagesOccurrencesOrderingAndSelector(t *testing.T) {
	t.Parallel()

	packageIn := func(name, displayName string) *testutil.Package {
		return &testutil.Package{
			LeafGraph:       &testutil.LeafGraph{GraphName: name},
			PackageMetadata: catalogv1.PackageMetadata{DisplayName: displayName, Provider: catalogv1.Provider{Name: "provider"}},
		}
	}
	alphaHighZ := packageIn("alpha", "high-z")
	alphaHighA := packageIn("alpha", "high-a")
	alphaLow := packageIn("alpha", "low")
	beta := packageIn("beta", "beta")
	excluded := packageIn("alpha", "excluded")
	catalogs := []*testutil.Catalog{
		{CatalogName: "z", CatalogPriority: 10, CatalogLabels: map[string]string{"env": "prod"}, Packages: map[string]catalogv1.Package{"alpha": alphaHighZ}},
		{CatalogName: "a", CatalogPriority: 10, CatalogLabels: map[string]string{"env": "prod"}, Packages: map[string]catalogv1.Package{"alpha": alphaHighA, "beta": beta}},
		{CatalogName: "low", CatalogPriority: 1, CatalogLabels: map[string]string{"env": "prod"}, Packages: map[string]catalogv1.Package{"alpha": alphaLow}},
		{CatalogName: "dev", CatalogPriority: 100, CatalogLabels: map[string]string{"env": "dev"}, Packages: map[string]catalogv1.Package{"alpha": excluded}},
	}
	reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalogs[0], catalogs[1], catalogs[2], catalogs[3]}}
	response := httptest.NewRecorder()
	NewHandler(reader).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/packages?catalogSelector=env%3Dprod", nil))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	var collection packageCollection
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &collection))
	require.Len(t, collection.Packages, 4)
	assert.Equal(t, []string{"a", "z", "low", "a"}, []string{
		collection.Packages[0].CatalogName,
		collection.Packages[1].CatalogName,
		collection.Packages[2].CatalogName,
		collection.Packages[3].CatalogName,
	})
	assert.Equal(t, []string{"alpha", "alpha", "alpha", "beta"}, []string{
		collection.Packages[0].Name,
		collection.Packages[1].Name,
		collection.Packages[2].Name,
		collection.Packages[3].Name,
	})
	assert.Equal(t, []string{"high-a", "high-z", "low", "beta"}, []string{
		collection.Packages[0].DisplayName,
		collection.Packages[1].DisplayName,
		collection.Packages[2].DisplayName,
		collection.Packages[3].DisplayName,
	})
	for _, catalog := range catalogs[:3] {
		assert.Equal(t, 1, catalog.ListPackagesCalls)
	}
	assert.Equal(t, 0, catalogs[3].ListPackagesCalls)
	for _, pkg := range []*testutil.Package{alphaHighZ, alphaHighA, alphaLow, beta} {
		assert.Equal(t, 1, pkg.MetadataCalls)
	}
	assert.Equal(t, 0, excluded.MetadataCalls)
	require.NotNil(t, reader.LastSelection)
	assert.Equal(t, 1, reader.LastSelection.ListCalls)
}

func TestListPackagesPaginationAndCursorBinding(t *testing.T) {
	t.Parallel()

	newPackage := func(name string) *testutil.Package {
		return &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: name}}
	}
	catalog := &testutil.Catalog{CatalogName: "catalog", CatalogDigest: "1", Packages: map[string]catalogv1.Package{
		"a": newPackage("a"),
		"b": newPackage("b"),
		"c": newPackage("c"),
	}}
	api := NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}})
	firstResponse := httptest.NewRecorder()
	api.ServeHTTP(firstResponse, httptest.NewRequest(http.MethodGet, "/v1/packages?limit=2", nil))
	var first packageCollection
	require.NoError(t, json.Unmarshal(firstResponse.Body.Bytes(), &first))
	require.Len(t, first.Packages, 2)
	assert.Equal(t, []string{"a", "b"}, []string{first.Packages[0].Name, first.Packages[1].Name})
	assert.Equal(t, 3, first.Total)
	assert.NotEmpty(t, first.NextCursor)

	continuedResponse := httptest.NewRecorder()
	api.ServeHTTP(continuedResponse, httptest.NewRequest(http.MethodGet, "/v1/packages?limit=200&cursor="+first.NextCursor, nil))
	assert.Equal(t, http.StatusOK, continuedResponse.Code)
	var continued packageCollection
	require.NoError(t, json.Unmarshal(continuedResponse.Body.Bytes(), &continued))
	require.Len(t, continued.Packages, 1)
	assert.Equal(t, "c", continued.Packages[0].Name)
	assert.Equal(t, 3, continued.Total)
	assert.Empty(t, continued.NextCursor)

	wrongSelector := httptest.NewRecorder()
	api.ServeHTTP(wrongSelector, httptest.NewRequest(http.MethodGet, "/v1/packages?catalogSelector=env%3Dprod&cursor="+first.NextCursor, nil))
	assert.Equal(t, http.StatusConflict, wrongSelector.Code)
	assertProblem(t, wrongSelector, problemStaleCursor)

	missingKey, err := encodeCursor(
		packageCursorKey{Name: "z", CatalogName: catalog.Name()},
		collectionRequest{}.normalizedInput("/v1/packages"),
		[]catalogv1.Catalog{catalog},
	)
	require.NoError(t, err)
	missingResponse := httptest.NewRecorder()
	api.ServeHTTP(missingResponse, httptest.NewRequest(http.MethodGet, "/v1/packages?cursor="+missingKey, nil))
	assert.Equal(t, http.StatusBadRequest, missingResponse.Code)
	assertProblem(t, missingResponse, problemInvalidCursor)
}

func TestListPackagesReadsEverythingBeforeSuccessHeaders(t *testing.T) {
	t.Parallel()

	first := &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "a"}}
	failing := &testutil.Package{
		LeafGraph:   &testutil.LeafGraph{GraphName: "b"},
		MetadataErr: errors.New("secret metadata failure"),
	}
	catalog := &testutil.Catalog{CatalogName: "catalog", Packages: map[string]catalogv1.Package{"a": first, "b": failing}}
	response := httptest.NewRecorder()
	NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/packages", nil))

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
	assertProblem(t, response, problemCatalogReadFailure)
	assert.NotContains(t, response.Body.String(), "secret")
	assert.Equal(t, 1, first.MetadataCalls)
	assert.Equal(t, 1, failing.MetadataCalls)
}

func TestListDiscoveryReadFailuresDoNotLeakDetails(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		path   string
		reader *testutil.StoreReader
	}{
		{name: "catalog list", path: "/v1/catalogs", reader: &testutil.StoreReader{ListErr: errors.New("secret list failure")}},
		{name: "package list", path: "/v1/packages", reader: &testutil.StoreReader{Catalogs: []catalogv1.Catalog{
			&testutil.Catalog{CatalogName: "catalog", ListPackagesErr: errors.New("secret package failure")},
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewHandler(test.reader).ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))

			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assertProblem(t, response, problemCatalogReadFailure)
			assert.NotContains(t, strings.ToLower(response.Body.String()), "secret")
			assert.Equal(t, 1, test.reader.ListCalls)
		})
	}
}
