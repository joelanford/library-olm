package catalogv1

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURL(t *testing.T) {
	for _, value := range []string{
		"https://example.com/packages?q=one",
		"http://localhost:8080/path",
	} {
		t.Run(value, func(t *testing.T) {
			got, err := ParseURL(value)
			require.NoError(t, err)
			assert.Equal(t, value, got.String())

			data, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, `"`+value+`"`, string(data))
		})
	}

	for _, value := range []string{
		"",
		"example.com/path",
		"ftp://example.com/path",
		"https:///path",
		"://invalid",
	} {
		t.Run("invalid "+value, func(t *testing.T) {
			_, err := ParseURL(value)
			assert.Error(t, err)
		})
	}
}

func TestURLUnmarshalJSON(t *testing.T) {
	var got URL
	require.NoError(t, json.Unmarshal([]byte(`"https://example.com/path"`), &got))
	assert.Equal(t, "https://example.com/path", got.String())
	assert.Error(t, json.Unmarshal([]byte(`"file:///tmp/icon.svg"`), &got))
}

func TestParseEmailAddress(t *testing.T) {
	for _, value := range []string{"person@example.com", "first.last+tag@example.co.uk"} {
		t.Run(value, func(t *testing.T) {
			got, err := ParseEmailAddress(value)
			require.NoError(t, err)
			assert.Equal(t, value, got.String())

			data, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, `"`+value+`"`, string(data))
		})
	}

	for _, value := range []string{"", "not-an-email", "Person <person@example.com>", "a@example.com, b@example.com"} {
		t.Run("invalid "+value, func(t *testing.T) {
			_, err := ParseEmailAddress(value)
			assert.Error(t, err)
		})
	}
}

func TestEmailAddressUnmarshalJSON(t *testing.T) {
	var got EmailAddress
	require.NoError(t, json.Unmarshal([]byte(`"person@example.com"`), &got))
	assert.Equal(t, "person@example.com", got.String())
	assert.Error(t, json.Unmarshal([]byte(`"Person <person@example.com>"`), &got))
}

func TestIconClose(t *testing.T) {
	wantErr := errors.New("close")
	content := &testReadCloser{Reader: strings.NewReader("icon"), closeErr: wantErr}
	icon := Icon{Content: content, MediaType: "image/svg+xml"}

	assert.ErrorIs(t, icon.Close(), wantErr)
	assert.True(t, content.closed)
	assert.NoError(t, (Icon{}).Close())
}

type testReadCloser struct {
	*strings.Reader
	closeErr error
	closed   bool
}

func (r *testReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}
