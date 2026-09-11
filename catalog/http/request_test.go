package cataloghttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	testutil "github.com/joelanford/library-olm/internal/util/test"
)

func TestParseCollectionRequest(t *testing.T) {
	t.Parallel()

	prod := &testutil.Catalog{CatalogName: "prod", CatalogLabels: map[string]string{"env": "prod"}}
	dev := &testutil.Catalog{CatalogName: "dev", CatalogLabels: map[string]string{"env": "dev"}}
	reader := &testutil.StoreReader{Catalogs: []catalogv1.Catalog{prod, dev}}

	t.Run("defaults and selector narrowing", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/v1/packages?catalogSelector=env%3Dprod", nil)
		parsed, selected, problem := parseCollectionRequest(request, reader, true)

		require.Nil(t, problem)
		assert.Equal(t, defaultLimit, parsed.Limit)
		assert.Equal(t, "env=prod", parsed.CatalogSelector)
		catalogs, err := selected.List()
		require.NoError(t, err)
		require.Len(t, catalogs, 1)
		assert.Equal(t, "prod", catalogs[0].Name())
	})

	t.Run("empty selector selects all", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/v1/packages?catalogSelector=&limit=200", nil)
		parsed, selected, problem := parseCollectionRequest(request, reader, true)

		require.Nil(t, problem)
		assert.Equal(t, maximumLimit, parsed.Limit)
		catalogs, err := selected.List()
		require.NoError(t, err)
		assert.Len(t, catalogs, 2)
	})

	for _, value := range []string{"0", "201", "x", "1.5", ""} {
		t.Run("invalid limit "+value, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/packages?limit="+value, nil)
			_, _, problem := parseCollectionRequest(request, reader, true)

			require.NotNil(t, problem)
			assert.Equal(t, problemMalformedInput.status, problem.Status)
		})
	}

	t.Run("selector forbidden on scoped collection", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/v1/catalogs/c/packages/p/channels?catalogSelector=env%3Dprod", nil)
		_, _, problem := parseCollectionRequest(request, reader, false)

		require.NotNil(t, problem)
		assert.Equal(t, problemMalformedInput.status, problem.Status)
	})
}

func TestDecodeJSONStrictness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		contentType string
		wantProblem problemKind
		wantOK      bool
	}{
		{name: "valid", body: `{"value":"ok"}`, contentType: "application/json; charset=utf-8", wantOK: true},
		{name: "wrong media type", body: `{}`, contentType: "text/json", wantProblem: problemUnsupportedMediaType},
		{name: "unknown field", body: `{"other":1}`, contentType: "application/json", wantProblem: problemMalformedInput},
		{name: "empty", contentType: "application/json", wantProblem: problemMalformedInput},
		{name: "multiple values", body: `{} {}`, contentType: "application/json", wantProblem: problemMalformedInput},
		{name: "malformed", body: `{`, contentType: "application/json", wantProblem: problemMalformedInput},
		{name: "null", body: `null`, contentType: "application/json", wantProblem: problemMalformedInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			var destination struct {
				Value string `json:"value"`
			}

			problem := decodeJSON(request, &destination)

			if test.wantOK {
				require.Nil(t, problem)
				assert.Equal(t, "ok", destination.Value)
				return
			}
			require.NotNil(t, problem)
			assert.Equal(t, test.wantProblem.status, problem.Status)
		})
	}

	t.Run("body limit", func(t *testing.T) {
		body := append([]byte(`{"value":"`), bytes.Repeat([]byte("x"), maximumBody)...)
		body = append(body, []byte(`"}`)...)
		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")

		problem := decodeJSON(request, &struct{}{})

		require.NotNil(t, problem)
		assert.Equal(t, problemMalformedInput.status, problem.Status)
	})
}

func TestDecodeRecommendationRequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		kind *problemKind
	}{
		{name: "defaults", body: `{}`},
		{name: "valid", body: `{"currentBundle":{"id":"p.v1.0.0","packageName":"p","version":"1.0.0","release":"1"},"channelPaths":[["stable"],["stable","1"]],"versionConstraint":">= 1.0.0","upgradeConstraintPolicy":"SelfCertified"}`},
		{name: "unsupported policy", body: `{"upgradeConstraintPolicy":"Any"}`, kind: &problemUnsupportedPolicy},
		{name: "wrong package", body: `{"currentBundle":{"id":"x","packageName":"other","version":"1.0.0","release":""}}`, kind: &problemMalformedInput},
		{name: "invalid version", body: `{"currentBundle":{"id":"x","packageName":"p","version":"latest","release":""}}`, kind: &problemMalformedInput},
		{name: "empty paths", body: `{"channelPaths":[]}`, kind: &problemMalformedInput},
		{name: "empty path segment", body: `{"channelPaths":[["stable",""]]}`, kind: &problemMalformedInput},
		{name: "colon path segment", body: `{"channelPaths":[["stable:1"]]}`, kind: &problemMalformedInput},
		{name: "duplicate paths", body: `{"channelPaths":[["stable"],["stable"]]}`, kind: &problemMalformedInput},
		{name: "invalid constraint", body: `{"versionConstraint":"not a constraint"}`, kind: &problemInvalidVersionConstraint},
		{name: "null request", body: `null`, kind: &problemMalformedInput},
		{name: "null current bundle", body: `{"currentBundle":null}`, kind: &problemMalformedInput},
		{name: "null current bundle release", body: `{"currentBundle":{"id":"p.v1.0.0","packageName":"p","version":"1.0.0","release":null}}`, kind: &problemMalformedInput},
		{name: "null channel paths", body: `{"channelPaths":null}`, kind: &problemMalformedInput},
		{name: "null version constraint", body: `{"versionConstraint":null}`, kind: &problemMalformedInput},
		{name: "null policy", body: `{"upgradeConstraintPolicy":null}`, kind: &problemMalformedInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			var decoded recommendationRequest

			problem := decodeRecommendationRequest(request, "p", &decoded)

			if test.kind == nil {
				require.Nil(t, problem)
				assert.NotEmpty(t, decoded.UpgradeConstraintPolicy)
				return
			}
			require.NotNil(t, problem)
			assert.Equal(t, test.kind.status, problem.Status)
			assert.Equal(t, problemTypeBase+test.kind.typeName, problem.Type)
		})
	}
}

func TestDecodeChannelPath(t *testing.T) {
	t.Parallel()

	path, problem := decodeChannelPath("stable:1:2")
	require.Nil(t, problem)
	assert.Equal(t, []string{"stable", "1", "2"}, path)

	for _, encoded := range []string{"", ":stable", "stable:", "stable::1"} {
		_, problem := decodeChannelPath(encoded)
		require.NotNil(t, problem)
		assert.Equal(t, problemMalformedInput.status, problem.Status)
	}
}
