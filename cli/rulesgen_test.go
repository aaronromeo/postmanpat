package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestConfig() *appconfig.Config {
	return &appconfig.Config{
		Rules: []appconfig.Rule{
			{
				Name: "Test Rule",
				Server: &appconfig.ServerMatchers{
					Folders: []string{"INBOX"},
				},
				Actions: []appconfig.Action{
					{Type: appconfig.DELETE},
				},
			},
		},
	}
}

func TestRulesgenServe_DefaultPort(t *testing.T) {
	// Create server with default port and test config
	cfg := createTestConfig()
	server, err := rulesgen.NewServer(8080, cfg)
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
	// Create server with custom port and test config
	customPort := 9999
	cfg := createTestConfig()
	server, err := rulesgen.NewServer(customPort, cfg)
	require.NoError(t, err)

	// Use httptest to avoid port conflicts
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Make request to verify server is running
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify response contains expected content
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "PostmanPat Rules Generator")
}
