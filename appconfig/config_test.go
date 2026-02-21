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
	t.Setenv(envS3Endpoint, "")
	t.Setenv(envS3Region, "")
	t.Setenv(envS3Bucket, "")
	t.Setenv(envS3Key, "")
	t.Setenv(envS3Secret, "")
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

func TestValidateMissingServerFolders(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server: {}
    actions: []
    archive:
      path_template: "archive/{date}"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for missing server.folders")
	} else if !strings.Contains(err.Error(), "server.folders") {
		t.Fatalf("expected server.folders error, got: %v", err)
	}
}

func TestHappyPath(t *testing.T) {
	t.Setenv(envIMAPHost, "imap.example.com")
	t.Setenv(envIMAPPort, "993")
	t.Setenv(envIMAPUser, "user@example.com")
	t.Setenv(envIMAPPass, "password")
	t.Setenv(envS3Endpoint, "https://nyc3.digitaloceanspaces.com")
	t.Setenv(envS3Region, "nyc3")
	t.Setenv(envS3Bucket, "postmanpat-archive")
	t.Setenv(envS3Key, "key")
	t.Setenv(envS3Secret, "secret")
	t.Setenv(envWebhookURL, "https://example.com/webhook")

	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
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
