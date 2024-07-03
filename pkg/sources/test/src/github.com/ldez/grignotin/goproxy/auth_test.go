package goproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBasicAuthTransport_success(t *testing.T) {
	username := "☺"
	password := "secret"

	transport, err := NewBasicAuthTransport(username, password)
	require.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestNewBasicAuthTransport_missing_credentials(t *testing.T) {
	username := ""
	password := ""

	transport, err := NewBasicAuthTransport(username, password)
	require.Error(t, err)
	assert.Nil(t, transport)
}

func TestTokenTransport_RoundTrip(t *testing.T) {
	username := "user"
	password := "secret"

	transport, err := NewBasicAuthTransport(username, password)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	user, pass, ok := resp.Request.BasicAuth()
	assert.True(t, ok)
	assert.Equal(t, "user", user)
	assert.Equal(t, "secret", pass)
}
