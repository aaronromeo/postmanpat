package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronromeo/postmanpat/internal/config"
)

func TestWatchRejectsServerMatchers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.SetArgs([]string{"watch", "--config", path})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected watch to fail with server matchers")
	}
	if !strings.Contains(err.Error(), "server matchers") {
		t.Fatalf("expected server matchers error, got: %v", err)
	}
}

func TestWatchAcceptsClientMatchers(t *testing.T) {
	cfg := config.Config{
		Rules: []config.Rule{
			{
				Name: "Rule",
				Client: &config.ClientMatchers{
					SubjectRegex: []string{"hello"},
				},
			},
		},
	}

	if err := validateWatchRules(cfg); err != nil {
		t.Fatalf("expected client matchers to be accepted, got: %v", err)
	}
}
