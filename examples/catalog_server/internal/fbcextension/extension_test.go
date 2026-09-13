package fbcextension_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/fbc"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/fbcextension"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
)

func TestExtensionImportsPortableMetadata(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	content := strings.Join([]string{
		`{"schema":"olm.package","name":"metadata-op","description":"Package description","icon":{"base64data":"aWNvbg==","mediatype":"image/png"}}`,
		`{"schema":"olm.channel","name":"stable","package":"metadata-op","entries":[{"name":"metadata-op.v1.0.0"},{"name":"metadata-op.v2.0.0"}]}`,
		bundleJSON("metadata-op.v1.0.0", "1.0.0", "Old name", "2025-01-02", "plain"),
		bundleJSON("metadata-op.v2.0.0", "2.0.0", "Selected name", "2026-09-11 12:30:00", "registry+v1"),
	}, "\n")

	catalog, err := store.Set(ctx, "test", catalogv1.WithURI("test://"), catalogv1.WithContent(
		fbc.NewReaderImporter(strings.NewReader(content), fbc.WithOLMPackageExtension(fbcextension.New())), "",
	))
	require.NoError(t, err)
	pkg, err := catalog.GetPackage(ctx, "metadata-op")
	require.NoError(t, err)

	metadata, err := model.PackageMetadataFrom(ctx, pkg)
	require.NoError(t, err)
	assert.Equal(t, "Selected name", metadata.DisplayName)
	assert.Equal(t, "Short description", metadata.ShortDescription)
	assert.Equal(t, "Package description", metadata.Description)
	assert.Equal(t, "Provider", metadata.Provider.Name)
	require.NotNil(t, metadata.Provider.URL)
	assert.Equal(t, "https://provider.example.com", metadata.Provider.URL.String())
	require.Len(t, metadata.Maintainers, 1)
	assert.Equal(t, "owner@example.com", metadata.Maintainers[0].Email.String())
	assert.Equal(t, []string{"database", "storage"}, metadata.Keywords)
	require.NotNil(t, metadata.SourceRepository)
	assert.Equal(t, "https://github.com/example/metadata-op", metadata.SourceRepository.String())
	assert.True(t, metadata.IconAvailable)

	icon, mediaType, err := model.IconFrom(ctx, pkg)
	require.NoError(t, err)
	require.NotNil(t, icon)
	defer func() { require.NoError(t, icon.Close()) }()
	assert.Equal(t, "image/png", mediaType)
	iconData, err := io.ReadAll(icon)
	require.NoError(t, err)
	assert.Equal(t, "icon", string(iconData))

	want := map[string]model.BundleMetadata{
		"metadata-op.v1.0.0": {MediaType: "plain", ReleaseTimestamp: timePointer(time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC))},
		"metadata-op.v2.0.0": {MediaType: "registry+v1", ReleaseTimestamp: timePointer(time.Date(2026, time.September, 11, 12, 30, 0, 0, time.UTC))},
	}
	for bundle, bundleErr := range pkg.ListBundles(ctx) {
		require.NoError(t, bundleErr)
		metadata, metadataErr := model.BundleMetadataFrom(ctx, bundle)
		require.NoError(t, metadataErr)
		assert.Equal(t, want[string(bundle.ID())], metadata)
	}
}

func TestImporterWithoutExtensionDoesNotWritePortableProperties(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	content := strings.Join([]string{
		`{"schema":"olm.package","name":"plain-op","description":"Description"}`,
		`{"schema":"olm.channel","name":"stable","package":"plain-op","entries":[{"name":"plain-op.v1.0.0"}]}`,
		bundleJSON("plain-op.v1.0.0", "1.0.0", "Display", "2026-09-11", "plain"),
	}, "\n")
	catalog, err := store.Set(ctx, "test", catalogv1.WithURI("test://"), catalogv1.WithContent(fbc.NewReaderImporter(strings.NewReader(content)), ""))
	require.NoError(t, err)
	pkg, err := catalog.GetPackage(ctx, "plain-op")
	require.NoError(t, err)
	raw, err := pkg.Property(ctx, model.PackageMetadataProperty)
	require.NoError(t, err)
	assert.Nil(t, raw)
	for bundle, bundleErr := range pkg.ListBundles(ctx) {
		require.NoError(t, bundleErr)
		raw, propertyErr := bundle.Property(ctx, model.BundleMetadataProperty)
		require.NoError(t, propertyErr)
		assert.Nil(t, raw)
	}
}

func TestExtensionRejectsInvalidPortableMetadata(t *testing.T) {
	for _, test := range []struct {
		name        string
		packageJSON string
		mutateCSV   func(map[string]any)
		createdAt   string
		mediaType   string
		want        string
	}{
		{name: "icon", packageJSON: `{"schema":"olm.package","name":"bad-op","icon":{"base64data":"aWNvbg==","mediatype":"text/plain"}}`, want: "icon media type"},
		{name: "provider URL", packageJSON: `{"schema":"olm.package","name":"bad-op"}`, mutateCSV: func(csv map[string]any) {
			csv["provider"] = map[string]string{"name": "Provider", "url": "ftp://example.com"}
		}, want: "provider URL"},
		{name: "maintainer email", packageJSON: `{"schema":"olm.package","name":"bad-op"}`, mutateCSV: func(csv map[string]any) {
			csv["maintainers"] = []map[string]string{{"name": "Owner", "email": "not-an-email"}}
		}, want: "email"},
		{name: "source repository", packageJSON: `{"schema":"olm.package","name":"bad-op"}`, mutateCSV: func(csv map[string]any) { csv["annotations"].(map[string]string)["repository"] = "not-a-url" }, want: "source repository"},
		{name: "media type", packageJSON: `{"schema":"olm.package","name":"bad-op"}`, mediaType: "unknown", want: "bundle media type"},
		{name: "timestamp", packageJSON: `{"schema":"olm.package","name":"bad-op"}`, createdAt: "yesterday", want: "release timestamp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			csv := csvMetadata(test.createdAt, "Bad", test.mediaType)
			if test.mutateCSV != nil {
				test.mutateCSV(csv)
			}
			content := strings.Join([]string{
				test.packageJSON,
				`{"schema":"olm.channel","name":"stable","package":"bad-op","entries":[{"name":"bad-op.v1.0.0"}]}`,
				bundleJSONWithCSV("bad-op.v1.0.0", "1.0.0", csv),
			}, "\n")
			store := openStore(t)
			_, err := store.Set(context.Background(), "test", catalogv1.WithURI("test://"), catalogv1.WithContent(
				fbc.NewReaderImporter(strings.NewReader(content), fbc.WithOLMPackageExtension(fbcextension.New())), "",
			))
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestExtensionValidationUsesPartialImportSemantics(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	content := strings.Join([]string{
		`{"schema":"olm.package","name":"good-op"}`,
		`{"schema":"olm.channel","name":"stable","package":"good-op","entries":[{"name":"good-op.v1.0.0"}]}`,
		bundleJSON("good-op.v1.0.0", "1.0.0", "Good", "", ""),
		`{"schema":"olm.package","name":"bad-op","icon":{"base64data":"aWNvbg==","mediatype":"text/plain"}}`,
		`{"schema":"olm.channel","name":"stable","package":"bad-op","entries":[{"name":"bad-op.v1.0.0"}]}`,
		bundleJSON("bad-op.v1.0.0", "1.0.0", "Bad", "", ""),
	}, "\n")

	catalog, err := store.Set(ctx, "test", catalogv1.WithURI("test://"), catalogv1.WithContent(
		fbc.NewReaderImporter(strings.NewReader(content), fbc.WithOLMPackageExtension(fbcextension.New())), "",
	))
	require.Error(t, err)
	var partial catalogv1.PartialImportError
	assert.True(t, errors.As(err, &partial))

	good, err := catalog.GetPackage(ctx, "good-op")
	require.NoError(t, err)
	metadata, err := model.PackageMetadataFrom(ctx, good)
	require.NoError(t, err)
	assert.Equal(t, "Good", metadata.DisplayName)

	_, err = catalog.GetPackage(ctx, "bad-op")
	assert.NoError(t, err, "core writes completed before finalization are not rolled back")
}

func TestExtensionRejectsColonInChannelName(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	content := strings.Join([]string{
		`{"schema":"olm.package","name":"colon-op"}`,
		`{"schema":"olm.channel","name":"stable:v1","package":"colon-op","entries":[{"name":"colon-op.v1.0.0"}]}`,
		bundleJSON("colon-op.v1.0.0", "1.0.0", "", "", ""),
	}, "\n")

	_, err := store.Set(ctx, "test", catalogv1.WithURI("test://"), catalogv1.WithContent(
		fbc.NewReaderImporter(strings.NewReader(content), fbc.WithOLMPackageExtension(fbcextension.New())), "",
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contains ':'")
}

func openStore(t *testing.T) catalogv1.Store {
	t.Helper()
	store, err := sqlite.OpenStore(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

func bundleJSON(name, version, displayName, createdAt, mediaType string) string {
	return bundleJSONWithCSV(name, version, csvMetadata(createdAt, displayName, mediaType))
}

func csvMetadata(createdAt, displayName, mediaType string) map[string]any {
	return map[string]any{
		"annotations": map[string]string{
			"description": "Short description",
			"repository":  "https://github.com/example/metadata-op",
			"createdAt":   createdAt,
			"operators.operatorframework.io.bundle.mediatype.v1": mediaType,
		},
		"description": "CSV description",
		"displayName": displayName,
		"provider":    map[string]string{"name": "Provider", "url": "https://provider.example.com"},
		"maintainers": []map[string]string{{"name": "Owner", "email": "owner@example.com"}},
		"keywords":    []string{"database", "storage"},
	}
}

func bundleJSONWithCSV(name, version string, csv map[string]any) string {
	bundle := map[string]any{
		"schema":  "olm.bundle",
		"name":    name,
		"package": strings.Split(name, ".v")[0],
		"image":   "quay.io/example/" + strings.Split(name, ".v")[0] + ":" + version,
		"properties": []any{
			map[string]any{"type": "olm.package", "value": map[string]string{"packageName": strings.Split(name, ".v")[0], "version": version}},
			map[string]any{"type": "olm.csv.metadata", "value": csv},
		},
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func timePointer(value time.Time) *time.Time { return &value }
