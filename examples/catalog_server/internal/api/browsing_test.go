package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
	testutil "github.com/joelanford/library-olm/examples/catalog_server/internal/testutil"
)

type deprecatedCompositeGraph struct {
	*testutil.CompositeGraph
	message string
}

func (g *deprecatedCompositeGraph) DeprecationMessage() string { return g.message }

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type failingReadCloser struct {
	read   bool
	closed bool
}

func (r *failingReadCloser) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("secret copy failure")
}

func (r *failingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestGetPackageDetailAndLookupOutcomes(t *testing.T) {
	t.Parallel()

	providerURL, err := model.ParseURL("https://provider.example.com")
	require.NoError(t, err)
	sourceURL, err := model.ParseURL("https://source.example.com/repo")
	require.NoError(t, err)
	email, err := model.ParseEmailAddress("owner@example.com")
	require.NoError(t, err)
	basePackage := &testutil.Package{
		LeafGraph: &testutil.LeafGraph{GraphName: "package/name"},
		PackageMetadata: model.PackageMetadata{
			DisplayName:      "Package Display",
			ShortDescription: "Short",
			Description:      "Long description",
			Provider:         model.Provider{Name: "Provider", URL: &providerURL},
			Maintainers:      []model.Maintainer{{Name: "Owner", Email: &email}},
			Keywords:         []string{"one", "two"},
			SourceRepository: &sourceURL,
			IconAvailable:    true,
		},
	}
	pkg := &deprecatedPackage{Package: basePackage, message: "use another package"}
	catalog := &testutil.Catalog{
		CatalogName:     "catalog name",
		CatalogPriority: 9,
		Packages:        map[string]catalogv1.UpdateGraph{"package/name": pkg},
	}
	response := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}), "/v1/catalogs/catalog%20name/packages/package%2Fname")

	assert.Equal(t, http.StatusOK, response.Code)
	var detail packageDetail
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &detail))
	assert.Equal(t, "catalog name", detail.CatalogName)
	assert.Equal(t, 9, detail.CatalogPriority)
	assert.Equal(t, "package/name", detail.Name)
	assert.Equal(t, "Package Display", detail.DisplayName)
	assert.Equal(t, "Long description", detail.Description)
	assert.Equal(t, "https://provider.example.com", *detail.Provider.URL)
	assert.Equal(t, "owner@example.com", *detail.Maintainers[0].Email)
	assert.Equal(t, "https://source.example.com/repo", *detail.SourceRepository)
	assert.Equal(t, "use another package", detail.DeprecationMessage)
	assert.Equal(t, "/v1/catalogs/catalog%20name/packages/package%2Fname", detail.Links.Self.Href)
	require.NotNil(t, detail.Links.Icon)
	assert.Equal(t, detail.Links.Self.Href+"/icon", detail.Links.Icon.Href)
	assert.Equal(t, 1, basePackage.MetadataCalls)
	assert.Equal(t, 0, basePackage.IconCalls, "package detail must not open the icon")
	assert.Equal(t, []string{"package/name"}, catalog.GetPackageCalls)

	tests := []struct {
		name    string
		reader  *testutil.StoreReader
		path    string
		status  int
		problem problemKind
	}{
		{name: "catalog absent", reader: &testutil.StoreReader{}, path: "/v1/catalogs/missing/packages/p", status: http.StatusNotFound, problem: problemNotFound},
		{name: "catalog read failure", reader: &testutil.StoreReader{GetErr: errors.New("secret catalog error")}, path: "/v1/catalogs/c/packages/p", status: http.StatusInternalServerError, problem: problemCatalogReadFailure},
		{name: "package absent", reader: &testutil.StoreReader{Catalogs: []catalogv1.Catalog{&testutil.Catalog{CatalogName: "c"}}}, path: "/v1/catalogs/c/packages/missing", status: http.StatusNotFound, problem: problemNotFound},
		{name: "package read failure", reader: &testutil.StoreReader{Catalogs: []catalogv1.Catalog{&testutil.Catalog{CatalogName: "c", GetPackageErr: errors.New("secret package error")}}}, path: "/v1/catalogs/c/packages/p", status: http.StatusInternalServerError, problem: problemCatalogReadFailure},
		{name: "metadata read failure", reader: &testutil.StoreReader{Catalogs: []catalogv1.Catalog{&testutil.Catalog{CatalogName: "c", Packages: map[string]catalogv1.UpdateGraph{"p": &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "p"}, MetadataErr: errors.New("secret metadata error")}}}}}, path: "/v1/catalogs/c/packages/p", status: http.StatusInternalServerError, problem: problemCatalogReadFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, NewHandler(test.reader), test.path)
			assert.Equal(t, test.status, response.Code)
			assertProblem(t, response, test.problem)
			assert.NotContains(t, response.Body.String(), "secret")
		})
	}
}

func TestChannelListRecursesSortsPaginatesAndDetectsCycles(t *testing.T) {
	t.Parallel()

	level2 := &testutil.LeafGraph{GraphName: "2"}
	level1 := &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "1"},
		Graphs:    map[string]catalogv1.UpdateGraph{"2": level2},
	}
	stable := &deprecatedCompositeGraph{
		CompositeGraph: &testutil.CompositeGraph{
			LeafGraph: &testutil.LeafGraph{GraphName: "stable"},
			Graphs: map[string]catalogv1.UpdateGraph{
				"2": &testutil.LeafGraph{GraphName: "2"},
				"1": level1,
			},
		},
		message: "stable is deprecated",
	}
	pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "package"},
		Graphs: map[string]catalogv1.UpdateGraph{
			"stable": stable,
			"beta":   &testutil.LeafGraph{GraphName: "beta"},
		},
	}}
	catalog := &testutil.Catalog{CatalogName: "catalog", CatalogDigest: "one", Packages: map[string]catalogv1.UpdateGraph{"package": pkg}}
	api := NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}})
	firstResponse := request(t, api, "/v1/catalogs/catalog/packages/package/channels?limit=3")

	assert.Equal(t, http.StatusOK, firstResponse.Code)
	var first channelCollection
	require.NoError(t, json.Unmarshal(firstResponse.Body.Bytes(), &first))
	assert.Equal(t, [][]string{{"beta"}, {"stable"}, {"stable", "1"}}, channelPaths(first.Channels))
	assert.Equal(t, "stable is deprecated", first.Channels[1].DeprecationMessage)
	assert.Equal(t, "/v1/catalogs/catalog/packages/package/channels/stable:1", first.Channels[2].Links.Self.Href)
	assert.Equal(t, 5, first.Total)
	assert.NotEmpty(t, first.NextCursor)

	continuedResponse := request(t, api, "/v1/catalogs/catalog/packages/package/channels?cursor="+first.NextCursor)
	assert.Equal(t, http.StatusOK, continuedResponse.Code)
	var continued channelCollection
	require.NoError(t, json.Unmarshal(continuedResponse.Body.Bytes(), &continued))
	assert.Equal(t, [][]string{{"stable", "1", "2"}, {"stable", "2"}}, channelPaths(continued.Channels))
	assert.Equal(t, 5, continued.Total)
	assert.Empty(t, continued.NextCursor)

	catalog.CatalogDigest = "two"
	stale := request(t, api, "/v1/catalogs/catalog/packages/package/channels?cursor="+first.NextCursor)
	assert.Equal(t, http.StatusConflict, stale.Code)
	assertProblem(t, stale, problemStaleCursor)

	cycle := &testutil.CompositeGraph{LeafGraph: &testutil.LeafGraph{GraphName: "cycle"}}
	cycle.Graphs = map[string]catalogv1.UpdateGraph{"cycle": cycle}
	cyclePackage := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "p"},
		Graphs:    map[string]catalogv1.UpdateGraph{"cycle": cycle},
	}}
	cycleCatalog := &testutil.Catalog{CatalogName: "c", Packages: map[string]catalogv1.UpdateGraph{"p": cyclePackage}}
	cycleResponse := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{cycleCatalog}}), "/v1/catalogs/c/packages/p/channels")
	assert.Equal(t, http.StatusInternalServerError, cycleResponse.Code)
	assertProblem(t, cycleResponse, problemCatalogReadFailure)

	failingChild := &testutil.CompositeGraph{LeafGraph: &testutil.LeafGraph{GraphName: "stable"}, ListGraphsErr: errors.New("secret nested error")}
	failingPackage := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "p"},
		Graphs:    map[string]catalogv1.UpdateGraph{"stable": failingChild},
	}}
	failingCatalog := &testutil.Catalog{CatalogName: "c", Packages: map[string]catalogv1.UpdateGraph{"p": failingPackage}}
	failure := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{failingCatalog}}), "/v1/catalogs/c/packages/p/channels")
	assert.Equal(t, http.StatusInternalServerError, failure.Code)
	assert.Equal(t, "application/problem+json", failure.Header().Get("Content-Type"))
	assert.NotContains(t, failure.Body.String(), "secret")
}

func TestChannelDetailWalksExactDecodedPath(t *testing.T) {
	t.Parallel()

	level2 := &testutil.LeafGraph{GraphName: "2"}
	level1 := &testutil.CompositeGraph{LeafGraph: &testutil.LeafGraph{GraphName: "1"}, Graphs: map[string]catalogv1.UpdateGraph{"2": level2}}
	stable := &testutil.CompositeGraph{LeafGraph: &testutil.LeafGraph{GraphName: "stable channel"}, Graphs: map[string]catalogv1.UpdateGraph{"1": level1}}
	pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "package/name"},
		Graphs:    map[string]catalogv1.UpdateGraph{"stable channel": stable},
	}}
	catalog := &testutil.Catalog{CatalogName: "catalog name", Packages: map[string]catalogv1.UpdateGraph{"package/name": pkg}}
	response := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}), "/v1/catalogs/catalog%20name/packages/package%2Fname/channels/stable%20channel:1:2")

	assert.Equal(t, http.StatusOK, response.Code)
	var detail channelDetail
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &detail))
	assert.Equal(t, []string{"stable channel", "1", "2"}, detail.Path)
	assert.Equal(t, "/v1/catalogs/catalog%20name/packages/package%2Fname/channels/stable%20channel:1:2", detail.Links.Self.Href)
	assert.Equal(t, []string{"stable channel"}, pkg.GetGraphCalls)
	assert.Equal(t, []string{"1"}, stable.GetGraphCalls)
	assert.Equal(t, []string{"2"}, level1.GetGraphCalls)
	assert.Zero(t, pkg.ListGraphsCalls)

	missing := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}), "/v1/catalogs/catalog%20name/packages/package%2Fname/channels/stable%20channel:missing")
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assertProblem(t, missing, problemNotFound)

	stable.GetGraphErr = errors.New("secret graph error")
	failure := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}), "/v1/catalogs/catalog%20name/packages/package%2Fname/channels/stable%20channel:1")
	assert.Equal(t, http.StatusInternalServerError, failure.Code)
	assertProblem(t, failure, problemCatalogReadFailure)
	assert.NotContains(t, failure.Body.String(), "secret")
}

func TestBundleCollectionsUseAddressedGraphSortAndBindCursors(t *testing.T) {
	t.Parallel()

	bundle1 := testutil.NewBundle(t, "package", "1.0.0", "2")
	bundle1.BundleURI = "oci://example/package:1.0.0-2"
	bundle2 := testutil.NewBundle(t, "package", "2.0.0", "")
	bundle2.BundleURI = "oci://example/package:2.0.0"
	bundle3 := testutil.NewBundle(t, "package", "1.0.0", "10")
	bundle3.BundleURI = "oci://example/package:1.0.0-10"
	channelBundle := testutil.NewBundle(t, "package", "3.0.0", "")
	channelBundle.BundleURI = "oci://example/package:3.0.0"
	child := &testutil.LeafGraph{GraphName: "child", Bundles: []bundlev1.Bundle{testutil.NewBundle(t, "package", "9.0.0", "")}}
	stable := &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "stable", Bundles: []bundlev1.Bundle{channelBundle}},
		Graphs:    map[string]catalogv1.UpdateGraph{"child": child},
	}
	pkg := &testutil.CompositePackage{CompositeGraph: &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "package", Bundles: []bundlev1.Bundle{bundle1, bundle2, bundle3}},
		Graphs:    map[string]catalogv1.UpdateGraph{"stable": stable},
	}}
	catalog := &testutil.Catalog{CatalogName: "catalog", CatalogDigest: "one", Packages: map[string]catalogv1.UpdateGraph{"package": pkg}}
	reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog, &testutil.Catalog{CatalogName: "unrelated", CatalogDigest: "old"}}}
	api := NewHandler(reader)

	firstResponse := request(t, api, "/v1/catalogs/catalog/packages/package/bundles?limit=2")
	assert.Equal(t, http.StatusOK, firstResponse.Code)
	var first bundleCollection
	require.NoError(t, json.Unmarshal(firstResponse.Body.Bytes(), &first))
	assert.Equal(t, []string{string(bundle2.ID()), string(bundle3.ID())}, bundleIDs(first.Bundles))
	assert.Equal(t, []string{bundle2.URI(), bundle3.URI()}, bundleURIs(first.Bundles))
	assert.Equal(t, 3, first.Total)
	assert.NotEmpty(t, first.NextCursor)
	assert.Equal(t, 1, pkg.ListBundlesCalls)
	assert.Zero(t, stable.ListBundlesCalls)
	assert.Zero(t, child.ListBundlesCalls)

	reader.Catalogs[1].(*testutil.Catalog).CatalogDigest = "changed"
	continuedResponse := request(t, api, "/v1/catalogs/catalog/packages/package/bundles?cursor="+first.NextCursor)
	assert.Equal(t, http.StatusOK, continuedResponse.Code, "unrelated catalogs are outside the scoped snapshot")
	var continued bundleCollection
	require.NoError(t, json.Unmarshal(continuedResponse.Body.Bytes(), &continued))
	assert.Equal(t, []string{string(bundle1.ID())}, bundleIDs(continued.Bundles))
	assert.Equal(t, []string{bundle1.URI()}, bundleURIs(continued.Bundles))
	assert.Equal(t, 3, continued.Total)

	wrongEndpoint := request(t, api, "/v1/catalogs/catalog/packages/package/channels/stable/bundles?cursor="+first.NextCursor)
	assert.Equal(t, http.StatusConflict, wrongEndpoint.Code)
	assertProblem(t, wrongEndpoint, problemStaleCursor)
	assert.Equal(t, 1, stable.ListBundlesCalls)
	assert.Zero(t, child.ListBundlesCalls, "composite behavior belongs to ListBundles")

	channelResponse := request(t, api, "/v1/catalogs/catalog/packages/package/channels/stable/bundles")
	assert.Equal(t, http.StatusOK, channelResponse.Code)
	var channelBundles bundleCollection
	require.NoError(t, json.Unmarshal(channelResponse.Body.Bytes(), &channelBundles))
	assert.Equal(t, []string{string(channelBundle.ID())}, bundleIDs(channelBundles.Bundles))
	assert.Equal(t, []string{channelBundle.URI()}, bundleURIs(channelBundles.Bundles))
	assert.Equal(t, 2, stable.ListBundlesCalls)
	assert.Zero(t, child.ListBundlesCalls)

	catalog.CatalogDigest = "two"
	stale := request(t, api, "/v1/catalogs/catalog/packages/package/bundles?cursor="+first.NextCursor)
	assert.Equal(t, http.StatusConflict, stale.Code)
	assertProblem(t, stale, problemStaleCursor)

	unknownQuery := request(t, api, "/v1/catalogs/catalog/packages/package/bundles?channelPath=stable")
	assert.Equal(t, http.StatusBadRequest, unknownQuery.Code)
	problem := assertProblem(t, unknownQuery, problemMalformedInput)
	assert.Equal(t, "channelPath", problem.InvalidParams[0].Name)
}

func TestBundleCollectionsBufferIteratorFailuresBeforeHeaders(t *testing.T) {
	t.Parallel()

	graph := &testutil.LeafGraph{
		GraphName:      "package",
		Bundles:        []bundlev1.Bundle{testutil.NewBundle(t, "package", "1.0.0", "")},
		ListBundlesErr: errors.New("secret bundle read error"),
	}
	pkg := &testutil.Package{LeafGraph: graph}
	catalog := &testutil.Catalog{CatalogName: "catalog", Packages: map[string]catalogv1.UpdateGraph{"package": pkg}}
	response := request(t, NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}}), "/v1/catalogs/catalog/packages/package/bundles")

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, "application/problem+json", response.Header().Get("Content-Type"))
	assertProblem(t, response, problemCatalogReadFailure)
	assert.NotContains(t, response.Body.String(), "secret")
	assert.Equal(t, 1, graph.ListBundlesCalls)
}

func TestGetBundleBuffersRootAndProjectsMetadata(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 9, 11, 12, 30, 0, 0, time.UTC)
	base := testutil.NewBundle(t, "package", "1.2.3", "4")
	base.BundleURI = "oci://bundle"
	base.BundleMetadata = model.BundleMetadata{MediaType: "registry+v1", ReleaseTimestamp: &timestamp}
	bundle := &testutil.DeprecatedBundle{Bundle: base, Message: "replace this bundle"}
	other := testutil.NewBundle(t, "package", "2.0.0", "")
	graph := &testutil.LeafGraph{GraphName: "package", Bundles: []bundlev1.Bundle{bundle, other}}
	pkg := &testutil.Package{LeafGraph: graph}
	catalog := &testutil.Catalog{CatalogName: "catalog", Packages: map[string]catalogv1.UpdateGraph{"package": pkg}}
	api := NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}})
	response := request(t, api, "/v1/catalogs/catalog/packages/package/bundles/"+string(bundle.ID()))

	assert.Equal(t, http.StatusOK, response.Code)
	var detail bundleDetail
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &detail))
	assert.Equal(t, string(bundle.ID()), detail.ID)
	assert.Equal(t, "package", detail.PackageName)
	assert.Equal(t, "1.2.3", detail.Version)
	assert.Equal(t, "4", detail.Release)
	assert.Equal(t, "oci://bundle", detail.URI)
	assert.Equal(t, "registry+v1", detail.MediaType)
	assert.Equal(t, timestamp, *detail.ReleaseTimestamp)
	assert.Equal(t, "replace this bundle", detail.DeprecationMessage)
	assert.Equal(t, 1, graph.ListBundlesCalls)
	assert.Equal(t, 1, base.MetadataCalls)
	assert.Zero(t, other.MetadataCalls)

	missing := request(t, api, "/v1/catalogs/catalog/packages/package/bundles/missing")
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assertProblem(t, missing, problemNotFound)

	graph.ListBundlesErr = errors.New("secret trailing iterator error")
	preHeaderFailure := request(t, api, "/v1/catalogs/catalog/packages/package/bundles/"+string(bundle.ID()))
	assert.Equal(t, http.StatusInternalServerError, preHeaderFailure.Code)
	assertProblem(t, preHeaderFailure, problemCatalogReadFailure)
	assert.Equal(t, 1, base.MetadataCalls, "metadata is not read before the root iterator is drained")

	graph.ListBundlesErr = nil
	base.MetadataErr = errors.New("secret metadata error")
	metadataFailure := request(t, api, "/v1/catalogs/catalog/packages/package/bundles/"+string(bundle.ID()))
	assert.Equal(t, http.StatusInternalServerError, metadataFailure.Code)
	assertProblem(t, metadataFailure, problemCatalogReadFailure)
	assert.NotContains(t, metadataFailure.Body.String(), "secret")
}

func TestGetIconStreamsValidContentAndHandlesFailures(t *testing.T) {
	t.Parallel()

	t.Run("streams and closes", func(t *testing.T) {
		pkg := &testutil.Package{
			LeafGraph:   &testutil.LeafGraph{GraphName: "package"},
			PackageIcon: model.Icon{Content: []byte("icon bytes"), MediaType: "image/svg+xml; charset=utf-8"},
		}
		response := request(t, handlerForPackage(pkg), "/v1/catalogs/catalog/packages/package/icon")
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "image/svg+xml; charset=utf-8", response.Header().Get("Content-Type"))
		assert.Equal(t, "attachment", response.Header().Get("Content-Disposition"))
		assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "default-src 'none'; sandbox", response.Header().Get("Content-Security-Policy"))
		assert.Equal(t, "icon bytes", response.Body.String())
		assert.Equal(t, 1, pkg.IconCalls)
		assert.Zero(t, pkg.MetadataCalls)
	})

	t.Run("writer streams and closes", func(t *testing.T) {
		stream := &trackingReadCloser{Reader: strings.NewReader("icon bytes")}
		response := httptest.NewRecorder()
		writeIcon(response, httptest.NewRequest(http.MethodGet, "/icon", nil), stream, "image/png")
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "icon bytes", response.Body.String())
		assert.True(t, stream.closed)
	})

	t.Run("absent", func(t *testing.T) {
		pkg := &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "package"}}
		response := request(t, handlerForPackage(pkg), "/v1/catalogs/catalog/packages/package/icon")
		assert.Equal(t, http.StatusNotFound, response.Code)
		assertProblem(t, response, problemNotFound)
		assert.Equal(t, 1, pkg.IconCalls)
	})

	t.Run("open error", func(t *testing.T) {
		pkg := &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "package"}, IconErr: errors.New("secret open error")}
		response := request(t, handlerForPackage(pkg), "/v1/catalogs/catalog/packages/package/icon")
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assertProblem(t, response, problemCatalogReadFailure)
		assert.NotContains(t, response.Body.String(), "secret")
	})

	t.Run("invalid media type closes before headers", func(t *testing.T) {
		stream := &trackingReadCloser{Reader: strings.NewReader("not written")}
		response := httptest.NewRecorder()
		writeIcon(response, httptest.NewRequest(http.MethodGet, "/icon", nil), stream, "not a media type")
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assertProblem(t, response, problemCatalogReadFailure)
		assert.True(t, stream.closed)
		assert.NotContains(t, response.Body.String(), "not written")
	})

	t.Run("copy error terminates after headers", func(t *testing.T) {
		stream := &failingReadCloser{}
		response := httptest.NewRecorder()
		writeIcon(response, httptest.NewRequest(http.MethodGet, "/icon", nil), stream, "image/png")
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "image/png", response.Header().Get("Content-Type"))
		assert.Equal(t, "partial", response.Body.String())
		assert.NotContains(t, response.Body.String(), problemTypeBase)
		assert.True(t, stream.closed)
	})
}

func request(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func handlerForPackage(pkg catalogv1.UpdateGraph) http.Handler {
	catalog := &testutil.Catalog{CatalogName: "catalog", Packages: map[string]catalogv1.UpdateGraph{"package": pkg}}
	return NewHandler(&testutil.StoreReader{Catalogs: []catalogv1.Catalog{catalog}})
}

func channelPaths(channels []channelSummary) [][]string {
	paths := make([][]string, len(channels))
	for i := range channels {
		paths[i] = channels[i].Path
	}
	return paths
}

func bundleIDs(bundles []bundleSummary) []string {
	ids := make([]string, len(bundles))
	for i := range bundles {
		ids[i] = bundles[i].ID
	}
	return ids
}

func bundleURIs(bundles []bundleSummary) []string {
	refs := make([]string, len(bundles))
	for i := range bundles {
		refs[i] = bundles[i].URI
	}
	return refs
}

var _ catalogv1.Deprecated = (*deprecatedCompositeGraph)(nil)
