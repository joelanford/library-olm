package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/examples/catalog_server/internal/model"
)

type propertyGraph struct {
	properties map[string]json.RawMessage
	err        error
}

func (g propertyGraph) Name() string { return "package" }
func (g propertyGraph) ListBundles(context.Context) iter.Seq2[bundlev1.Bundle, error] {
	return func(func(bundlev1.Bundle, error) bool) {}
}
func (g propertyGraph) Successors(context.Context, bundlev1.BundleIdentity) iter.Seq2[bundlev1.Bundle, error] {
	return func(func(bundlev1.Bundle, error) bool) {}
}
func (g propertyGraph) Property(_ context.Context, key string) (json.RawMessage, error) {
	return g.properties[key], g.err
}

type propertyBundle struct {
	propertyGraph
}

func (propertyBundle) ID() bundlev1.BundleID { return "package.v1.0.0" }
func (propertyBundle) NameVersionRelease() bundlev1.NameVersionRelease {
	return bundlev1.NameVersionRelease{Name: "package"}
}
func (propertyBundle) URI() string { return "docker://example.com/package:v1.0.0" }

func TestPropertyDecoding(t *testing.T) {
	ctx := context.Background()
	released := time.Date(2026, time.September, 11, 12, 30, 0, 0, time.UTC)
	graph := propertyGraph{properties: map[string]json.RawMessage{
		model.PackageMetadataProperty: json.RawMessage(`{"displayName":"Package","provider":{"name":"Provider","url":"https://example.com"}}`),
		model.PackageIconProperty:     json.RawMessage(`{"content":"aWNvbg==","mediaType":"image/png"}`),
		model.BundleMetadataProperty:  json.RawMessage(`{"mediaType":"registry+v1","releaseTimestamp":"2026-09-11T12:30:00Z"}`),
	}}

	metadata, err := model.PackageMetadataFrom(ctx, graph)
	require.NoError(t, err)
	assert.Equal(t, "Package", metadata.DisplayName)
	require.NotNil(t, metadata.Provider.URL)
	assert.Equal(t, "https://example.com", metadata.Provider.URL.String())

	icon, mediaType, err := model.IconFrom(ctx, graph)
	require.NoError(t, err)
	require.NotNil(t, icon)
	t.Cleanup(func() { require.NoError(t, icon.Close()) })
	assert.Equal(t, "image/png", mediaType)

	bundleMetadata, err := model.BundleMetadataFrom(ctx, propertyBundle{propertyGraph: graph})
	require.NoError(t, err)
	assert.Equal(t, "registry+v1", bundleMetadata.MediaType)
	assert.Equal(t, released, *bundleMetadata.ReleaseTimestamp)
}

func TestPropertyAbsenceAndFailures(t *testing.T) {
	ctx := context.Background()
	metadata, err := model.PackageMetadataFrom(ctx, propertyGraph{})
	require.NoError(t, err)
	assert.Equal(t, model.PackageMetadata{}, metadata)

	icon, mediaType, err := model.IconFrom(ctx, propertyGraph{})
	require.NoError(t, err)
	assert.Nil(t, icon)
	assert.Empty(t, mediaType)

	for _, test := range []struct {
		name  string
		graph propertyGraph
	}{
		{name: "malformed JSON", graph: propertyGraph{properties: map[string]json.RawMessage{model.PackageMetadataProperty: json.RawMessage(`{`)}}},
		{name: "null JSON", graph: propertyGraph{properties: map[string]json.RawMessage{model.PackageMetadataProperty: json.RawMessage(`null`)}}},
		{name: "invalid URL", graph: propertyGraph{properties: map[string]json.RawMessage{model.PackageMetadataProperty: json.RawMessage(`{"provider":{"url":"ftp://example.com"}}`)}}},
		{name: "read error", graph: propertyGraph{err: errors.New("read failed")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := model.PackageMetadataFrom(ctx, test.graph)
			assert.Error(t, err)
		})
	}
}

func TestValidation(t *testing.T) {
	_, err := model.ParseURL("ftp://example.com")
	assert.Error(t, err)
	_, err = model.ParseEmailAddress("Name <name@example.com>")
	assert.Error(t, err)
	assert.Error(t, model.ValidateIcon(model.Icon{MediaType: "text/plain"}))
	assert.Error(t, model.ValidateIcon(model.Icon{MediaType: "image/*"}))
	assert.Error(t, model.ValidateBundleMetadata(model.BundleMetadata{MediaType: "unknown"}))
}
