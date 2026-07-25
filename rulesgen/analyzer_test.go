package rulesgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		wantErr bool
		errMsg  string
	}{
		{
			name: "all required options set",
			options: []Option{
				WithRulesConfig("/path/to/config.yaml"),
				WithRulesGenStore("/path/to/store.db"),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
			},
			wantErr: false,
		},
		{
			name:    "no options set",
			options: []Option{},
			wantErr: true,
			errMsg:  "config path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewServerConfig(tt.options...)

			if !tt.wantErr {
				assert.Equal(t, "/path/to/config.yaml", config.ConfigPath)
				assert.Equal(t, "/path/to/store.db", config.StorePath)
				assert.Equal(t, "/path/to/watch.yml", config.WatchOut)
				assert.Equal(t, "/path/to/cleanup.yml", config.CleanupOut)
				assert.Equal(t, "/path/to/onetime.yml", config.OnetimeOut)
			}
		})
	}
}

func TestServerConfig_Validate(t *testing.T) {
	// Create a temporary config file for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	storePath := filepath.Join(tmpDir, "test-store.db")

	// Write a valid config file
	configContent := `
rules:
  - name: "Test Rule"
    server:
      folders:
        - "INBOX"
    actions:
      - type: delete
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Write a store file
	err = os.WriteFile(storePath, []byte("test"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name    string
		options []Option
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: false,
		},
		{
			name: "missing config path",
			options: []Option{
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "config path is required",
		},
		{
			name: "missing store path",
			options: []Option{
				WithRulesConfig(configPath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "rulesgen store path is required",
		},
		{
			name: "missing watch out",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "watch out path is required",
		},
		{
			name: "missing cleanup out",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "cleanup out path is required",
		},
		{
			name: "missing onetime out",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "onetime cleanup out path is required",
		},
		{
			name: "missing port",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithAddr("imap.gmail.com"),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "web server port is required",
		},
		{
			name: "missing address",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithCreds("uszzzer", "passssssss"),
			},
			wantErr: true,
			errMsg:  "imap address is required",
		},
		{
			name: "missing creds",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("", "passssssss"),
			},
			wantErr: true,
			errMsg:  "imap username is required",
		},
		{
			name: "missing creds",
			options: []Option{
				WithRulesConfig(configPath),
				WithRulesGenStore(storePath),
				WithWatchOut("/path/to/watch.yml"),
				WithCleanupOut("/path/to/cleanup.yml"),
				WithOneTimeOut("/path/to/onetime.yml"),
				WithPort(1234),
				WithAddr("imap.gmail.com"),
				WithCreds("usadfasd", ""),
			},
			wantErr: true,
			errMsg:  "imap password is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewServerConfig(tt.options...)
			err := config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultAnalyzeOptions(t *testing.T) {
	opts := DefaultAnalyzeOptions()
	assert.Equal(t, 100, opts.Top)
	assert.Equal(t, 20, opts.Examples)
	assert.Equal(t, 2, opts.MinCount)
}
