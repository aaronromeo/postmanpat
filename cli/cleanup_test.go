package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupRejectsClientMatchers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    client:
      subject_regex:
        - "hello"
    actions: []
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.SetArgs([]string{"cleanup", "--config", path})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected cleanup to fail with client matchers")
	}
	if !strings.Contains(err.Error(), "client matchers") {
		t.Fatalf("expected client matchers error, got: %v", err)
	}
}
