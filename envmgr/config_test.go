package envmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRequiredEnvVars() []string {
	return []string{
		EnvIMAPHost,
		EnvIMAPPort,
		EnvIMAPUser,
		EnvIMAPPass,
		EnvS3Endpoint,
		EnvS3Region,
		EnvS3Bucket,
		EnvS3Key,
		EnvS3Secret,
	}
}

func TestValidateEnvMissing(t *testing.T) {
	t.Setenv(EnvIMAPHost, "")
	t.Setenv(EnvIMAPPort, "")
	t.Setenv(EnvIMAPUser, "")
	t.Setenv(EnvIMAPPass, "")
	t.Setenv(EnvS3Endpoint, "")
	t.Setenv(EnvS3Region, "")
	t.Setenv(EnvS3Bucket, "")
	t.Setenv(EnvS3Key, "")
	t.Setenv(EnvS3Secret, "")
	t.Setenv(EnvWebhookURL, "")

	if err := ValidateEnv(testRequiredEnvVars); err == nil {
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
	t.Setenv(EnvIMAPHost, "imap.example.com")
	t.Setenv(EnvIMAPPort, "993")
	t.Setenv(EnvIMAPUser, "user@example.com")
	t.Setenv(EnvIMAPPass, "password")
	t.Setenv(EnvS3Endpoint, "https://nyc3.digitaloceanspaces.com")
	t.Setenv(EnvS3Region, "nyc3")
	t.Setenv(EnvS3Bucket, "postmanpat-archive")
	t.Setenv(EnvS3Key, "key")
	t.Setenv(EnvS3Secret, "secret")
	t.Setenv(EnvWebhookURL, "https://example.com/webhook")

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

	if err := ValidateEnv(testRequiredEnvVars); err != nil {
		t.Fatalf("expected env validation to pass, got error: %v", err)
	}
}

func writeTempFile(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "appconfig.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}
