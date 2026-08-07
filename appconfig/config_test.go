package appconfig

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
	path := filepath.Join(dir, "appconfig.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestIgnoreSectionAbsentIsValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore != nil {
		t.Fatalf("expected Ignore to be nil when section absent, got: %+v", cfg.Ignore)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass with no ignore section, got: %v", err)
	}
}

func TestIgnoreSectionParsesBothSubsections(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
    sender_addresses:
      - "noreply@github.com"
    list_ids:
      - "github-notifications"
    recipient_tags:
      - "dev"
    subject_substrings:
      - "[GitHub]"
  cleanup:
    sender_domains:
      - "mailer.example.com"
    sender_addresses:
      - "bulk@mailer.example.com"
    list_ids:
      - "weekly-digest"
    recipient_tags:
      - "promo"
    subject_substrings:
      - "Newsletter"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}

	w := cfg.Ignore.Watch
	if len(w.SenderDomains) != 1 || w.SenderDomains[0] != "github.com" {
		t.Fatalf("watch.sender_domains: got %v, want [github.com]", w.SenderDomains)
	}
	if len(w.SenderAddresses) != 1 || w.SenderAddresses[0] != "noreply@github.com" {
		t.Fatalf("watch.sender_addresses: got %v", w.SenderAddresses)
	}
	if len(w.ListIDs) != 1 || w.ListIDs[0] != "github-notifications" {
		t.Fatalf("watch.list_ids: got %v", w.ListIDs)
	}
	if len(w.RecipientTags) != 1 || w.RecipientTags[0] != "dev" {
		t.Fatalf("watch.recipient_tags: got %v", w.RecipientTags)
	}
	if len(w.SubjectSubstrings) != 1 || w.SubjectSubstrings[0] != "[GitHub]" {
		t.Fatalf("watch.subject_substrings: got %v", w.SubjectSubstrings)
	}

	c := cfg.Ignore.Cleanup
	if len(c.SenderDomains) != 1 || c.SenderDomains[0] != "mailer.example.com" {
		t.Fatalf("cleanup.sender_domains: got %v", c.SenderDomains)
	}
	if len(c.SenderAddresses) != 1 || c.SenderAddresses[0] != "bulk@mailer.example.com" {
		t.Fatalf("cleanup.sender_addresses: got %v", c.SenderAddresses)
	}
	if len(c.ListIDs) != 1 || c.ListIDs[0] != "weekly-digest" {
		t.Fatalf("cleanup.list_ids: got %v", c.ListIDs)
	}
	if len(c.RecipientTags) != 1 || c.RecipientTags[0] != "promo" {
		t.Fatalf("cleanup.recipient_tags: got %v", c.RecipientTags)
	}
	if len(c.SubjectSubstrings) != 1 || c.SubjectSubstrings[0] != "Newsletter" {
		t.Fatalf("cleanup.subject_substrings: got %v", c.SubjectSubstrings)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}

func TestIgnoreEmptyListsAreValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch: {}
  cleanup: {}
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass with empty lists, got: %v", err)
	}
}

func TestIgnoreWhitespaceOnlyEntryInvalid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
      - "   "
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for whitespace-only entry")
	} else if !strings.Contains(err.Error(), "sender_domains") {
		t.Fatalf("expected error to mention 'sender_domains', got: %v", err)
	}
}

func TestIgnoreOnlyWatchPopulatedIsValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}
	if len(cfg.Ignore.Watch.SenderDomains) != 1 {
		t.Fatal("expected watch.sender_domains to have 1 entry")
	}
	if len(cfg.Ignore.Cleanup.SenderDomains) != 0 {
		t.Fatalf("expected cleanup.sender_domains to be empty, got %v", cfg.Ignore.Cleanup.SenderDomains)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}

func TestIgnoreOnlyCleanupPopulatedIsValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  cleanup:
    list_ids:
      - "weekly-digest"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}
	if len(cfg.Ignore.Cleanup.ListIDs) != 1 {
		t.Fatal("expected cleanup.list_ids to have 1 entry")
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}
