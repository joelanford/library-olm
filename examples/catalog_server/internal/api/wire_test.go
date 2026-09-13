package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
	testutil "github.com/joelanford/library-olm/examples/catalog_server/internal/testutil"
)

type deprecatedPackage struct {
	*testutil.Package
	message string
}

func (p *deprecatedPackage) DeprecationMessage() string { return p.message }

func TestWireProjection(t *testing.T) {
	t.Parallel()

	providerURL, err := model.ParseURL("https://provider.example.com")
	require.NoError(t, err)
	sourceURL, err := model.ParseURL("https://source.example.com/repo")
	require.NoError(t, err)
	email, err := model.ParseEmailAddress("maintainer@example.com")
	require.NoError(t, err)

	catalog := &testutil.Catalog{
		CatalogName:     "red hat/catalog",
		CatalogURI:      "oci://catalog",
		CatalogDigest:   "sha256:123",
		CatalogPriority: 10,
		CatalogLabels:   map[string]string{"environment": "production"},
	}
	pkg := &deprecatedPackage{
		Package: &testutil.Package{
			LeafGraph: &testutil.LeafGraph{GraphName: "cert manager"},
			PackageMetadata: model.PackageMetadata{
				DisplayName:      "Certificate Manager",
				ShortDescription: "Manages certificates",
				Description:      "A complete description",
				Provider:         model.Provider{Name: "Provider", URL: &providerURL},
				Maintainers:      []model.Maintainer{{Name: "Maintainer", Email: &email}},
				Keywords:         []string{"certificates", "security"},
				SourceRepository: &sourceURL,
				IconAvailable:    true,
			},
		},
		message: "use replacement",
	}

	summary := catalogSummaryFrom("", catalog)
	assert.Equal(t, "/v1/catalogs/red%20hat%2Fcatalog", summary.Links.Self.Href)
	assert.Equal(t, catalog.CatalogLabels, summary.Labels)

	detail := packageDetailFrom("", catalog, pkg, pkg.PackageMetadata)
	assert.Equal(t, "cert manager", detail.Name)
	assert.Equal(t, "https://provider.example.com", *detail.Provider.URL)
	assert.Equal(t, "maintainer@example.com", *detail.Maintainers[0].Email)
	assert.Equal(t, "https://source.example.com/repo", *detail.SourceRepository)
	assert.Equal(t, "use replacement", detail.DeprecationMessage)
	require.NotNil(t, detail.Links.Icon)
	assert.Equal(t, "/v1/catalogs/red%20hat%2Fcatalog/packages/cert%20manager/icon", detail.Links.Icon.Href)

	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "PackageMetadata")
}

func TestChannelPathProjection(t *testing.T) {
	t.Parallel()

	graph := &testutil.LeafGraph{GraphName: "1/2"}
	detail := channelDetailFrom("", "catalog", "package", []string{"stable channel", "1/2"}, graph)
	assert.Equal(t, []string{"stable channel", "1/2"}, detail.Path)
	assert.Equal(t, "/v1/catalogs/catalog/packages/package/channels/stable%20channel:1%2F2", detail.Links.Self.Href)
}

func TestEscapeDotPathSegments(t *testing.T) {
	assert.Equal(t, "%2E", escapePathPart("."))
	assert.Equal(t, "%2E%2E", escapePathPart(".."))
}

func TestBundleProjection(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	bundle := testutil.NewBundle(t, "package", "1.2.3", "4")
	bundle.BundleURI = "oci://bundle"
	bundle.BundleMetadata = model.BundleMetadata{MediaType: "registry+v1", ReleaseTimestamp: &timestamp}

	detail, err := bundleDetailFrom(context.Background(), "", "catalog", "package", bundle)
	require.NoError(t, err)
	assert.Equal(t, string(bundle.ID()), detail.ID)
	assert.Equal(t, "1.2.3", detail.Version)
	assert.Equal(t, "4", detail.Release)
	assert.Equal(t, "oci://bundle", detail.URI)
	assert.Equal(t, "registry+v1", detail.MediaType)
	assert.Equal(t, timestamp, *detail.ReleaseTimestamp)
	assert.Equal(t, "/v1/catalogs/catalog/packages/package/bundles/package.v1.2.3-4", detail.Links.Self.Href)
}

func TestBundleSummaryProjection(t *testing.T) {
	t.Parallel()

	bundle := testutil.NewBundle(t, "package", "1.2.3", "4")
	bundle.BundleURI = "oci://registry.example/package@sha256:1234"

	summary := bundleSummaryFrom("", "catalog", "package", bundle)

	assert.Equal(t, "oci://registry.example/package@sha256:1234", summary.URI)
}

func TestRequiredCollectionsMarshalAsArrays(t *testing.T) {
	t.Parallel()

	values := []struct {
		value any
		field string
	}{
		{value: catalogCollection{Catalogs: []catalogSummary{}}, field: `"catalogs":[]`},
		{value: packageCollection{Packages: []packageSummary{}}, field: `"packages":[]`},
		{value: channelCollection{Channels: []channelSummary{}}, field: `"channels":[]`},
		{value: bundleCollection{Bundles: []bundleSummary{}}, field: `"bundles":[]`},
		{value: recommendation{Candidates: []bundleSummary{}}, field: `"candidates":[]`},
	}
	for _, value := range values {
		encoded, err := json.Marshal(value.value)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), value.field)
		assert.Contains(t, string(encoded), `"total":0`)
	}
}

var _ catalogv1.UpdateGraph = (*deprecatedPackage)(nil)
var _ catalogv1.Deprecated = (*deprecatedPackage)(nil)
