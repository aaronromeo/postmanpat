package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRulesgenServe_DefaultPort(t *testing.T) {
	// Create server with default port (will use httptest instead of actual port)
	server, err := rulesgen.NewServer(8080)
	require.NoError(t, err)

	// Use httptest to avoid port conflicts
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Make request to verify server is running
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))

	// Verify response contains expected content
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "PostmanPat Rules Generator")
	assert.Contains(t, string(body), "Server is running")
}

func TestRulesgenServe_CustomPort(t *testing.T) {
	// Create server with custom port (will use httptest instead of actual port)
	customPort := 9999
	server, err := rulesgen.NewServer(customPort)
	require.NoError(t, err)

	// Use httptest to avoid port conflicts
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Make request to verify server is running
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))

	// Verify response contains expected content
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "PostmanPat Rules Generator")
	assert.Contains(t, string(body), "Server is running")
}
