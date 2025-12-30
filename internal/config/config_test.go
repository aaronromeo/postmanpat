package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnvMissing(t *testing.T) {
	t.Setenv(envIMAPHost, "")
	t.Setenv(envIMAPPort, "")
	t.Setenv(envIMAPUser, "")
	t.Setenv(envIMAPPass, "")
	t.Setenv(envDOEndpoint, "")
	t.Setenv(envDORegion, "")
	t.Setenv(envDOBucket, "")
	t.Setenv(envDOKey, "")
	t.Setenv(envDOSecret, "")
	t.Setenv(envWebhookURL, "")

	if err := ValidateEnv(); err == nil {
		t.Fatalf("expected error for missing environment variables")
	} else if err != nil && !strings.Contains(err.Error(), "missing required environment variables") {
		t.Fatalf("expected missing env var error, got: %v", err)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTempFile(t, "not: [valid_yaml")
	if _, err := Load(path); err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

func TestValidateMissingRules(t *testing.T) {
	path := writeTempFile(t, `
rules: []
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for missing rules")
	}
}

func TestValidateMissingMatcherFolders(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    matchers: {}
    actions: []
    archive:
      path_template: "archive/{date}"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for missing matchers.folders")
	} else if !strings.Contains(err.Error(), "matchers.folders") {
		t.Fatalf("expected matchers.folders error, got: %v", err)
	}
}

func TestHappyPath(t *testing.T) {
	t.Setenv(envIMAPHost, "imap.example.com")
	t.Setenv(envIMAPPort, "993")
	t.Setenv(envIMAPUser, "user@example.com")
	t.Setenv(envIMAPPass, "password")
	t.Setenv(envDOEndpoint, "https://nyc3.digitaloceanspaces.com")
	t.Setenv(envDORegion, "nyc3")
	t.Setenv(envDOBucket, "postmanpat-archive")
	t.Setenv(envDOKey, "key")
	t.Setenv(envDOSecret, "secret")
	t.Setenv(envWebhookURL, "https://example.com/webhook")

	path := writeTempFile(t, `
rules:
  - name: "Rule"
    matchers:
      folders:
        - "INBOX"
    actions: []
    archive:
      path_template: "archive/{date}"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected config to validate, got error: %v", err)
	}

	if err := ValidateEnv(); err != nil {
		t.Fatalf("expected env validation to pass, got error: %v", err)
	}
}

func writeTempFile(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}
