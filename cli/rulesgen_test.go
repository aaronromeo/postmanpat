package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aaronromeo/postmanpat/envmgr"
	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestConfig() *envmgr.RulesConfig {
	return &envmgr.RulesConfig{
		Rules: []envmgr.Rule{
			{
				Name: "Test Rule",
				Server: &envmgr.ServerMatchers{
					Folders: []string{"INBOX"},
				},
				Actions: []envmgr.Action{
					{Type: envmgr.DELETE},
				},
			},
		},
	}
}

func TestRulesgenServe_DefaultPort(t *testing.T) {
	// Create server with default port, test config, and mock store
	cfg := createTestConfig()
	mockStore := rulesgen.NewMockStore()
	defer mockStore.Close()

	srvrCfg := &rulesgen.ServerConfig{
		Port: 8080,
		Cfg:  *cfg,
	}
	server, err := rulesgen.NewServerWithStore(srvrCfg, mockStore)
	require.NoError(t, err)
	defer server.Close()

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
	// Create server with custom port, test config, and mock store
	customPort := 9999
	cfg := createTestConfig()
	mockStore := rulesgen.NewMockStore()
	defer mockStore.Close()

	srvrCfg := &rulesgen.ServerConfig{
		Port: customPort,
		Cfg:  *cfg,
	}
	server, err := rulesgen.NewServerWithStore(srvrCfg, mockStore)
	require.NoError(t, err)
	defer server.Close()

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
