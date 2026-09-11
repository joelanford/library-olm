package cataloghttp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	testutil "github.com/joelanford/library-olm/internal/util/test"
)

func TestCursorCompactShapeAndRoundTrip(t *testing.T) {
	t.Parallel()

	input := map[string]any{"selector": "environment=production", "large": strings.Repeat("request-input", 100)}
	catalogs := []catalogv1.Catalog{&testutil.Catalog{
		CatalogName:     "production-catalog-with-a-long-name",
		CatalogDigest:   strings.Repeat("digest", 100),
		CatalogPriority: 12,
		CatalogLabels:   map[string]string{"environment": "production"},
	}}
	type stableKey struct {
		Name     string `json:"n"`
		Priority int    `json:"p"`
	}
	wantKey := stableKey{Name: "package-a", Priority: 12}

	token, err := encodeCursor(wantKey, input, catalogs)
	require.NoError(t, err)
	payload, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	assert.Equal(t, 2*cursorHashBytes+len(`{"n":"package-a","p":12}`), len(payload))
	assert.NotContains(t, string(payload), "request-input")
	assert.NotContains(t, string(payload), "production-catalog")
	assert.NotContains(t, string(payload), "digest")

	var gotKey stableKey
	require.NoError(t, validateCursor(token, input, catalogs, &gotKey))
	assert.Equal(t, wantKey, gotKey)
}

func TestCursorBindsInputAndCatalogSnapshot(t *testing.T) {
	t.Parallel()

	base := &testutil.Catalog{
		CatalogName:     "a",
		CatalogDigest:   "sha256:1",
		CatalogPriority: 10,
		CatalogLabels:   map[string]string{"a": "1", "b": "2"},
	}
	input := struct {
		Selector string `json:"selector"`
	}{Selector: "a=1"}
	token, err := encodeCursor("last", input, []catalogv1.Catalog{base})
	require.NoError(t, err)

	var key string
	assert.ErrorIs(t, validateCursor(token, struct {
		Selector string `json:"selector"`
	}{Selector: "b=2"}, []catalogv1.Catalog{base}, &key), errStaleCursor)

	changes := []*testutil.Catalog{
		{CatalogName: "renamed", CatalogDigest: base.CatalogDigest, CatalogPriority: base.CatalogPriority, CatalogLabels: base.CatalogLabels},
		{CatalogName: base.CatalogName, CatalogDigest: "sha256:2", CatalogPriority: base.CatalogPriority, CatalogLabels: base.CatalogLabels},
		{CatalogName: base.CatalogName, CatalogDigest: base.CatalogDigest, CatalogPriority: 11, CatalogLabels: base.CatalogLabels},
		{CatalogName: base.CatalogName, CatalogDigest: base.CatalogDigest, CatalogPriority: base.CatalogPriority, CatalogLabels: map[string]string{"a": "changed", "b": "2"}},
	}
	for _, changed := range changes {
		assert.ErrorIs(t, validateCursor(token, input, []catalogv1.Catalog{changed}, &key), errStaleCursor)
	}
}

func TestCollectionCursorInputNormalizationExcludesPagination(t *testing.T) {
	t.Parallel()

	first := collectionRequest{CatalogSelector: "a=1", Cursor: "first", Limit: 10}
	continuation := collectionRequest{CatalogSelector: "a=1", Cursor: "second", Limit: 200}
	firstHash, err := hashJSON(first.normalizedInput("packages"))
	require.NoError(t, err)
	continuationHash, err := hashJSON(continuation.normalizedInput("packages"))
	require.NoError(t, err)
	differentResourceHash, err := hashJSON(continuation.normalizedInput("catalogs"))
	require.NoError(t, err)

	assert.Equal(t, firstHash, continuationHash)
	assert.NotEqual(t, firstHash, differentResourceHash)
}

func TestCatalogSnapshotHashIsCanonical(t *testing.T) {
	t.Parallel()

	one := &testutil.Catalog{CatalogName: "one", CatalogDigest: "1", CatalogPriority: 1, CatalogLabels: map[string]string{"b": "2", "a": "1"}}
	two := &testutil.Catalog{CatalogName: "two", CatalogDigest: "2", CatalogPriority: 2, CatalogLabels: map[string]string{"d": "4", "c": "3"}}
	oneReordered := &testutil.Catalog{CatalogName: "one", CatalogDigest: "1", CatalogPriority: 1, CatalogLabels: map[string]string{"a": "1", "b": "2"}}
	twoReordered := &testutil.Catalog{CatalogName: "two", CatalogDigest: "2", CatalogPriority: 2, CatalogLabels: map[string]string{"c": "3", "d": "4"}}

	first, err := catalogSnapshotHash([]catalogv1.Catalog{two, one})
	require.NoError(t, err)
	second, err := catalogSnapshotHash([]catalogv1.Catalog{oneReordered, twoReordered})
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestCursorRejectsMalformedTokensSeparatelyFromStaleness(t *testing.T) {
	t.Parallel()

	for _, token := range []string{"", "%%%", base64.RawURLEncoding.EncodeToString(make([]byte, 64)), base64.RawURLEncoding.EncodeToString(append(make([]byte, 64), '{'))} {
		_, err := decodeCursor(token)
		assert.ErrorIs(t, err, errInvalidCursor)
		problem := cursorProblem(err)
		assert.Equal(t, problemInvalidCursor.status, problem.Status)
	}

	stale := cursorProblem(errStaleCursor)
	assert.Equal(t, problemStaleCursor.status, stale.Status)
	assert.False(t, errors.Is(errStaleCursor, errInvalidCursor))
}

func TestCursorRejectsKeyOfWrongShape(t *testing.T) {
	t.Parallel()

	token, err := encodeCursor(map[string]string{"name": "a"}, "input", nil)
	require.NoError(t, err)
	var key []string
	err = validateCursor(token, "input", nil, &key)
	assert.ErrorIs(t, err, errInvalidCursor)

	decoded, err := decodeCursor(token)
	require.NoError(t, err)
	assert.True(t, json.Valid(decoded.key))
}
