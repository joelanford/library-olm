package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	bsemver "github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
	testutil "github.com/joelanford/library-olm/examples/catalog_server/internal/testutil"
)

func TestRecommendMapsPolicyAndCurrentBundle(t *testing.T) {
	t.Parallel()

	currentBody := `"currentBundle":{"id":"pkg.v1.0.0","packageName":"pkg","version":"1.0.0","release":""}`
	tests := []struct {
		name               string
		body               string
		wantID             string
		wantListCalls      int
		wantSuccessorCalls int
	}{
		{
			name:               "catalog provided uses successors",
			body:               `{` + currentBody + `}`,
			wantID:             "pkg.v2.0.0",
			wantSuccessorCalls: 1,
		},
		{
			name:          "self certified lists bundles",
			body:          `{` + currentBody + `,"upgradeConstraintPolicy":"SelfCertified"}`,
			wantID:        "pkg.v3.0.0",
			wantListCalls: 1,
		},
		{
			name:          "install lists bundles",
			body:          `{}`,
			wantID:        "pkg.v3.0.0",
			wantListCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			listed := testutil.NewBundle(t, "pkg", "3.0.0", "")
			successor := testutil.NewBundle(t, "pkg", "2.0.0", "")
			graph := &testutil.LeafGraph{
				GraphName:        "pkg",
				Bundles:          []bundlev1.Bundle{listed},
				SuccessorBundles: []bundlev1.Bundle{successor},
			}
			pkg := &testutil.Package{LeafGraph: graph}
			api := NewHandler(recommendationReader(&testutil.Catalog{
				CatalogName: "catalog",
				Packages:    map[string]catalogv1.UpdateGraph{"pkg": pkg},
			}))

			response := postRecommendation(api, "/v1/recommendations/pkg", test.body)

			assert.Equal(t, http.StatusOK, response.Code)
			result := decodeRecommendation(t, response)
			require.Len(t, result.Candidates, 1)
			assert.Equal(t, test.wantID, result.Candidates[0].ID)
			assert.Equal(t, test.wantListCalls, graph.ListBundlesCalls)
			assert.Len(t, graph.SuccessorsCalls, test.wantSuccessorCalls)
			if test.wantSuccessorCalls != 0 {
				from := graph.SuccessorsCalls[0]
				assert.Equal(t, bundlev1.BundleID("pkg.v1.0.0"), from.ID())
				assert.Equal(t, "pkg", from.NameVersionRelease().Name)
				assert.Equal(t, bsemver.MustParse("1.0.0"), from.NameVersionRelease().Version)
			}
		})
	}
}

func TestRecommendTranslatesChannelsVersionAndDeprecationPreference(t *testing.T) {
	t.Parallel()

	deprecated := &testutil.DeprecatedBundle{Bundle: testutil.NewBundle(t, "pkg", "3.0.0", ""), Message: "deprecated"}
	nonDeprecated := testutil.NewBundle(t, "pkg", "2.0.0", "")
	tooOld := testutil.NewBundle(t, "pkg", "1.0.0", "")
	other := testutil.NewBundle(t, "pkg", "9.0.0", "")
	stable := &testutil.LeafGraph{GraphName: "stable", Bundles: []bundlev1.Bundle{tooOld, deprecated, nonDeprecated}}
	root := &testutil.CompositeGraph{
		LeafGraph: &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{other}},
		Graphs:    map[string]catalogv1.UpdateGraph{"stable": stable},
	}
	pkg := &testutil.CompositePackage{CompositeGraph: root}
	api := NewHandler(recommendationReader(&testutil.Catalog{
		CatalogName: "catalog",
		Packages:    map[string]catalogv1.UpdateGraph{"pkg": pkg},
	}))

	response := postRecommendation(api, "/v1/recommendations/pkg", `{
		"channelPaths":[["stable"]],
		"versionConstraint":">= 2.0.0",
		"upgradeConstraintPolicy":"SelfCertified"
	}`)

	assert.Equal(t, http.StatusOK, response.Code)
	result := decodeRecommendation(t, response)
	assert.Equal(t, []string{"pkg.v2.0.0", "pkg.v3.0.0"}, recommendationIDs(result.Candidates))
	assert.Equal(t, 0, root.ListBundlesCalls)
	assert.Equal(t, []string{"stable"}, root.GetGraphCalls)
	assert.Equal(t, 1, stable.ListBundlesCalls)
}

func TestRecommendIncludesCandidateBundleRef(t *testing.T) {
	t.Parallel()

	bundle := testutil.NewBundle(t, "pkg", "1.0.0", "")
	bundle.BundleURI = "oci://registry.example/pkg@sha256:1234"
	api := NewHandler(recommendationReader(&testutil.Catalog{
		CatalogName: "catalog",
		Packages: map[string]catalogv1.UpdateGraph{
			"pkg": &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{bundle}}},
		},
	}))

	response := postRecommendation(api, "/v1/recommendations/pkg", `{}`)

	assert.Equal(t, http.StatusOK, response.Code)
	result := decodeRecommendation(t, response)
	require.Len(t, result.Candidates, 1)
	assert.Equal(t, "oci://registry.example/pkg@sha256:1234", result.Candidates[0].URI)
}

func TestRecommendUsesSelectedReaderAndResultMetadata(t *testing.T) {
	t.Parallel()

	highPackage := &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{testutil.NewBundle(t, "pkg", "9.0.0", "")}}}
	lowPackage := &testutil.Package{
		LeafGraph:       &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{testutil.NewBundle(t, "pkg", "1.0.0", "")}},
		PackageMetadata: model.PackageMetadata{DisplayName: "selected package", Provider: model.Provider{Name: "provider"}},
	}
	high := &testutil.Catalog{
		CatalogName:     "high",
		CatalogPriority: 100,
		CatalogLabels:   map[string]string{"tier": "high"},
		Packages:        map[string]catalogv1.UpdateGraph{"pkg": highPackage},
	}
	low := &testutil.Catalog{
		CatalogName:     "low",
		CatalogPriority: 1,
		CatalogLabels:   map[string]string{"tier": "low"},
		Packages:        map[string]catalogv1.UpdateGraph{"pkg": lowPackage},
	}
	reader := recommendationReader(high, low)
	api := NewHandler(reader)

	response := postRecommendation(api, "/v1/recommendations/pkg?catalogSelector=tier%3Dlow", `{}`)

	assert.Equal(t, http.StatusOK, response.Code)
	result := decodeRecommendation(t, response)
	assert.Equal(t, "low", result.Catalog.Name)
	assert.Equal(t, "selected package", result.Package.DisplayName)
	assert.Equal(t, []string{"pkg.v1.0.0"}, recommendationIDs(result.Candidates))
	assert.Empty(t, high.GetPackageCalls)
	assert.Equal(t, []string{"pkg"}, low.GetPackageCalls)
	assert.Equal(t, 1, lowPackage.MetadataCalls)
	assert.Equal(t, []string{"tier=low"}, reader.SelectCalls)
	require.NotNil(t, reader.LastSelection)
	assert.Equal(t, 1, reader.LastSelection.ListCalls)
}

func TestRecommendMapsResolverOutcomesWithoutLeakingDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reader *testutil.StoreReader
		kind   problemKind
	}{
		{
			name:   "package absent",
			reader: recommendationReader(&testutil.Catalog{CatalogName: "catalog"}),
			kind:   problemNotFound,
		},
		{
			name: "ambiguous",
			reader: recommendationReader(
				&testutil.Catalog{CatalogName: "a", Packages: map[string]catalogv1.UpdateGraph{"pkg": testPackage("pkg")}},
				&testutil.Catalog{CatalogName: "b", Packages: map[string]catalogv1.UpdateGraph{"pkg": testPackage("pkg")}},
			),
			kind: problemAmbiguousPackage,
		},
		{
			name: "catalog list failure",
			reader: &testutil.StoreReader{
				ListErr: errors.New("secret list failure"),
			},
			kind: problemCatalogReadFailure,
		},
		{
			name: "package read failure",
			reader: recommendationReader(&testutil.Catalog{
				CatalogName:   "catalog",
				GetPackageErr: errors.New("secret package failure"),
			}),
			kind: problemCatalogReadFailure,
		},
		{
			name: "typed absence surfaced by resolver",
			reader: recommendationReader(&testutil.Catalog{
				CatalogName: "catalog",
				Packages: map[string]catalogv1.UpdateGraph{"pkg": &testutil.Package{LeafGraph: &testutil.LeafGraph{
					GraphName:      "pkg",
					ListBundlesErr: catalogv1.ErrNotFound,
				}}},
			}),
			kind: problemNotFound,
		},
		{
			name: "metadata failure",
			reader: recommendationReader(&testutil.Catalog{
				CatalogName: "catalog",
				Packages: map[string]catalogv1.UpdateGraph{"pkg": &testutil.Package{
					LeafGraph:   &testutil.LeafGraph{GraphName: "pkg"},
					MetadataErr: errors.New("secret metadata failure"),
				}},
			}),
			kind: problemCatalogReadFailure,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postRecommendation(NewHandler(test.reader), "/v1/recommendations/pkg", `{}`)

			assert.Equal(t, test.kind.status, response.Code)
			assertProblem(t, response, test.kind)
			assert.NotContains(t, strings.ToLower(response.Body.String()), "secret")
		})
	}
}

func TestRecommendRejectsMalformedInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		kind        problemKind
	}{
		{name: "unknown query", path: "/v1/recommendations/pkg?channel=stable", body: `{}`, contentType: "application/json", kind: problemMalformedInput},
		{name: "invalid limit", path: "/v1/recommendations/pkg?limit=0", body: `{}`, contentType: "application/json", kind: problemMalformedInput},
		{name: "invalid selector", path: "/v1/recommendations/pkg?catalogSelector=bad!", body: `{}`, contentType: "application/json", kind: problemInvalidCatalogSelector},
		{name: "malformed JSON", path: "/v1/recommendations/pkg", body: `{`, contentType: "application/json", kind: problemMalformedInput},
		{name: "unknown body field", path: "/v1/recommendations/pkg", body: `{"limit":1}`, contentType: "application/json", kind: problemMalformedInput},
		{name: "invalid channel", path: "/v1/recommendations/pkg", body: `{"channelPaths":[["stable:1"]]}`, contentType: "application/json", kind: problemMalformedInput},
		{name: "invalid version constraint", path: "/v1/recommendations/pkg", body: `{"versionConstraint":"nope"}`, contentType: "application/json", kind: problemInvalidVersionConstraint},
		{name: "unsupported policy", path: "/v1/recommendations/pkg", body: `{"upgradeConstraintPolicy":"Other"}`, contentType: "application/json", kind: problemUnsupportedPolicy},
		{name: "unsupported media type", path: "/v1/recommendations/pkg", body: `{}`, contentType: "text/plain", kind: problemUnsupportedMediaType},
	}
	api := NewHandler(&testutil.StoreReader{})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()

			api.ServeHTTP(response, request)

			assert.Equal(t, test.kind.status, response.Code)
			assertProblem(t, response, test.kind)
		})
	}
}

func TestRecommendPaginationBindsBodySelectorAndSnapshot(t *testing.T) {
	t.Parallel()

	bundleWithID := func(id, version string) bundlev1.Bundle {
		return &testutil.Bundle{BundleIdentity: testutil.BundleIdentity{
			BundleID: bundlev1.BundleID(id),
			NVR: bundlev1.NameVersionRelease{
				Name:    "pkg",
				Version: bsemver.MustParse(version),
			},
		}}
	}
	firstEqual := bundleWithID("first-equal", "2.0.0")
	secondEqual := bundleWithID("second-equal", "2.0.0")
	last := bundleWithID("last", "1.0.0")
	graph := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{last, firstEqual, secondEqual}}
	catalog := &testutil.Catalog{
		CatalogName:   "catalog",
		CatalogDigest: "one",
		CatalogLabels: map[string]string{"env": "prod", "region": "us"},
		Packages:      map[string]catalogv1.UpdateGraph{"pkg": &testutil.Package{LeafGraph: graph}},
	}
	reader := recommendationReader(catalog)
	api := NewHandler(reader)
	body := `{"upgradeConstraintPolicy":"SelfCertified","versionConstraint":">=1.0.0"}`
	selector := "env=prod,region=us"
	first := postRecommendation(api, "/v1/recommendations/pkg?catalogSelector="+url.QueryEscape(selector)+"&limit=1", body)
	assert.Equal(t, http.StatusOK, first.Code)
	firstPage := decodeRecommendation(t, first)
	assert.Equal(t, []string{"first-equal"}, recommendationIDs(firstPage.Candidates))
	assert.Equal(t, 3, firstPage.Total)
	assert.NotEmpty(t, firstPage.NextCursor)
	require.NotNil(t, reader.LastSelection)
	assert.Equal(t, 1, reader.LastSelection.ListCalls, "the selected snapshot is listed once")

	continuedPath := "/v1/recommendations/pkg?catalogSelector=" + url.QueryEscape("region=us,env=prod") + "&limit=2&cursor=" + firstPage.NextCursor
	continued := postRecommendation(api, continuedPath, body)
	assert.Equal(t, http.StatusOK, continued.Code)
	continuedPage := decodeRecommendation(t, continued)
	assert.Equal(t, []string{"second-equal", "last"}, recommendationIDs(continuedPage.Candidates))
	assert.Equal(t, 3, continuedPage.Total)
	assert.Empty(t, continuedPage.NextCursor)
	require.NotNil(t, reader.LastSelection)
	assert.Equal(t, 1, reader.LastSelection.ListCalls, "continuation validation and resolution reuse one snapshot")

	changedBody := postRecommendation(api, continuedPath, `{"upgradeConstraintPolicy":"SelfCertified","versionConstraint":">=2.0.0"}`)
	assert.Equal(t, http.StatusConflict, changedBody.Code)
	assertProblem(t, changedBody, problemStaleCursor)

	changedSelectorPath := "/v1/recommendations/pkg?catalogSelector=env%3Dprod&cursor=" + firstPage.NextCursor
	changedSelector := postRecommendation(api, changedSelectorPath, body)
	assert.Equal(t, http.StatusConflict, changedSelector.Code)
	assertProblem(t, changedSelector, problemStaleCursor)

	catalog.CatalogDigest = "two"
	stale := postRecommendation(api, continuedPath, body)
	assert.Equal(t, http.StatusConflict, stale.Code)
	assertProblem(t, stale, problemStaleCursor)
}

func TestRecommendSnapshotReadFailureOccursBeforeResolverExecution(t *testing.T) {
	t.Parallel()

	graph := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{
		testutil.NewBundle(t, "pkg", "2.0.0", ""),
		testutil.NewBundle(t, "pkg", "1.0.0", ""),
	}}
	pkg := &testutil.Package{LeafGraph: graph}
	reader := recommendationReader(&testutil.Catalog{
		CatalogName: "catalog",
		Packages:    map[string]catalogv1.UpdateGraph{"pkg": pkg},
	})
	reader.ListErr = errors.New("secret snapshot failure")

	response := postRecommendation(NewHandler(reader), "/v1/recommendations/pkg?limit=1", `{}`)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assertProblem(t, response, problemCatalogReadFailure)
	assert.NotContains(t, response.Body.String(), "secret")
	assert.Zero(t, graph.ListBundlesCalls)
	assert.Empty(t, reader.Catalogs[0].(*testutil.Catalog).GetPackageCalls)
}

func TestRecommendRejectsStaleCursorBeforeReplacementOutcomes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		replace func(*testutil.StoreReader, *testutil.Catalog)
	}{
		{
			name: "package removed",
			replace: func(_ *testutil.StoreReader, catalog *testutil.Catalog) {
				catalog.CatalogDigest = "two"
				catalog.Packages = nil
			},
		},
		{
			name: "package becomes ambiguous",
			replace: func(reader *testutil.StoreReader, catalog *testutil.Catalog) {
				catalog.CatalogDigest = "two"
				reader.Catalogs = append(reader.Catalogs, &testutil.Catalog{
					CatalogName:   "other",
					CatalogDigest: "other",
					Packages:      map[string]catalogv1.UpdateGraph{"pkg": testPackage("pkg")},
				})
			},
		},
		{
			name: "package reads fail",
			replace: func(_ *testutil.StoreReader, catalog *testutil.Catalog) {
				catalog.CatalogDigest = "two"
				catalog.GetPackageErr = errors.New("replacement read failure")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := &testutil.LeafGraph{GraphName: "pkg", Bundles: []bundlev1.Bundle{
				testutil.NewBundle(t, "pkg", "2.0.0", ""),
				testutil.NewBundle(t, "pkg", "1.0.0", ""),
			}}
			catalog := &testutil.Catalog{
				CatalogName:   "catalog",
				CatalogDigest: "one",
				Packages:      map[string]catalogv1.UpdateGraph{"pkg": &testutil.Package{LeafGraph: graph}},
			}
			reader := recommendationReader(catalog)
			api := NewHandler(reader)
			first := postRecommendation(api, "/v1/recommendations/pkg?limit=1", `{}`)
			require.Equal(t, http.StatusOK, first.Code)
			cursor := decodeRecommendation(t, first).NextCursor
			require.NotEmpty(t, cursor)
			packageCalls := len(catalog.GetPackageCalls)
			bundleCalls := graph.ListBundlesCalls

			test.replace(reader, catalog)
			continued := postRecommendation(api, "/v1/recommendations/pkg?cursor="+cursor, `{}`)

			assert.Equal(t, http.StatusConflict, continued.Code)
			assertProblem(t, continued, problemStaleCursor)
			assert.Len(t, catalog.GetPackageCalls, packageCalls)
			assert.Equal(t, bundleCalls, graph.ListBundlesCalls)
			if len(reader.Catalogs) > 1 {
				assert.Empty(t, reader.Catalogs[1].(*testutil.Catalog).GetPackageCalls)
			}
		})
	}
}

func recommendationReader(catalogs ...catalogv1.Catalog) *testutil.StoreReader {
	return &testutil.StoreReader{Catalogs: catalogs}
}

func testPackage(name string) catalogv1.UpdateGraph {
	return &testutil.Package{LeafGraph: &testutil.LeafGraph{GraphName: name}}
}

func postRecommendation(api http.Handler, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func decodeRecommendation(t *testing.T, response *httptest.ResponseRecorder) recommendation {
	t.Helper()
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	var result recommendation
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	return result
}

func recommendationIDs(candidates []bundleSummary) []string {
	ids := make([]string, len(candidates))
	for i, candidate := range candidates {
		ids[i] = candidate.ID
	}
	return ids
}
