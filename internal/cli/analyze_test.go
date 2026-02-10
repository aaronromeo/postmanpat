package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imap"
)

func TestBuildAnalyzeReportJSON(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	ageWindow := &config.AgeWindow{
		Min: "48h",
	}
	data := []imap.MailData{
		{
			SenderDomains:          []string{"example.com"},
			ReplyToDomains:         []string{"reply.example.com"},
			MailedByDomain:         "srs.messagingengine.com",
			Recipients:             []string{"me@example.com"},
			ListID:                 "list.example.com",
			ListUnsubscribe:        true,
			ListUnsubscribeTargets: "mailto:unsubscribe@example.com",
			PrecedenceRaw:          "bulk",
			PrecedenceCategory:     "bulk",
			XMailer:                "Mailer",
			UserAgent:              "Agent",
			SubjectRaw:             "Hello 2024",
			SubjectNormalized:      "hello {{n}}",
			MessageDate:            time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
		},
	}

	report, err := buildAnalyzeReport(data, analyzeReportParams{
		Mailbox:   "INBOX",
		Account:   "user@example.com",
		Generated: now,
		AgeWindow: ageWindow,
		Options: analyzeOptions{
			Top:      100,
			Examples: 20,
			MinCount: 1,
		},
	})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}

	path, err := writeAnalyzeReport(report)
	if err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if decoded["generated_at"] == "" {
		t.Fatal("missing generated_at")
	}

	source, ok := decoded["source"].(map[string]any)
	if !ok {
		t.Fatal("source is not an object")
	}
	if source["mailbox"] != "INBOX" {
		t.Fatalf("unexpected mailbox: %v", source["mailbox"])
	}

	timeWindow, ok := source["time_window"].(map[string]any)
	if !ok {
		t.Fatal("time_window is not an object")
	}
	if timeWindow["after"] == "" || timeWindow["before"] == "" {
		t.Fatal("time_window missing after or before")
	}

	stats, ok := decoded["stats"].(map[string]any)
	if !ok {
		t.Fatal("stats is not an object")
	}
	if stats["total_messages_scanned"] == nil {
		t.Fatal("missing total_messages_scanned")
	}

	indexes, ok := decoded["indexes"].(map[string]any)
	if !ok {
		t.Fatal("indexes is not an object")
	}
	// raw, ok := indexes["raw"].([]any)
	// if !ok {
	// 	t.Fatal("indexes.raw is not an array")
	// }
	// if len(raw) != 1 {
	// 	t.Fatalf("expected 1 raw record, got %d", len(raw))
	// }

	listLens, ok := indexes["list_lens"].(map[string]any)
	if !ok {
		t.Fatal("indexes.list_lens is not an object")
	}
	if listLens["key_fields"] == nil {
		t.Fatal("list_lens.key_fields is missing")
	}
	if listLens["clusters"] == nil {
		t.Fatal("list_lens.clusters is missing")
	}
	clusters, ok := listLens["clusters"].([]any)
	if !ok || len(clusters) == 0 {
		t.Fatal("list_lens.clusters is empty or invalid")
	}
	cluster, ok := clusters[0].(map[string]any)
	if !ok {
		t.Fatal("list_lens cluster is not an object")
	}
	if cluster["latest_date"] != "2024-01-10T12:00:00Z" {
		t.Fatalf("unexpected latest_date: %v", cluster["latest_date"])
	}
	if indexes["sender_lens"] == nil {
		t.Fatal("indexes.sender_lens is missing")
	}
	if indexes["template_lens"] == nil {
		t.Fatal("indexes.template_lens is missing")
	}
	if indexes["recipient_tag_lens"] == nil {
		t.Fatal("indexes.recipient_tag_lens is missing")
	}
	if indexes["mailedby_lens"] == nil {
		t.Fatal("indexes.mailedby_lens is missing")
	}

	// record, ok := raw[0].(map[string]any)
	// if !ok {
	// 	t.Fatal("raw record is not an object")
	// }
	// if record["SenderDomains"] != "example.com" {
	// 	t.Fatalf("unexpected SenderDomains: %v", record["SenderDomains"])
	// }
	// if record["SubjectNormalized"] != "hello {{n}}" {
	// 	t.Fatalf("unexpected SubjectNormalized: %v", record["SubjectNormalized"])
	// }
}

func TestBuildTimeWindow(t *testing.T) {
	now := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	window, err := buildTimeWindow(now, &config.AgeWindow{Min: "0h"})
	if err != nil {
		t.Fatalf("build time window: %v", err)
	}
	if window.Before == "" {
		t.Fatal("expected before timestamp")
	}
	if window.After == "" {
		t.Fatal("expected after timestamp with age_window set")
	}

	window, err = buildTimeWindow(now, nil)
	if err != nil {
		t.Fatalf("build time window: %v", err)
	}
	if window.Before == "" {
		t.Fatal("expected before timestamp")
	}
	if window.After == "" {
		t.Fatal("expected default after when age_window is nil")
	}
}

func TestBuildTimeWindowMax(t *testing.T) {
	now := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)

	window, err := buildTimeWindow(now, &config.AgeWindow{Max: "24h"})
	if err != nil {
		t.Fatalf("build time window: %v", err)
	}
	if window.After == "" {
		t.Fatal("expected default after when only max is set")
	}
	if window.Before != "2024-02-01T12:00:00Z" {
		t.Fatalf("unexpected before value: %q", window.Before)
	}
}

func TestBuildTimeWindowMin(t *testing.T) {
	now := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)

	window, err := buildTimeWindow(now, &config.AgeWindow{Min: "6h"})
	if err != nil {
		t.Fatalf("build time window: %v", err)
	}
	if window.Before == "" {
		t.Fatal("expected before timestamp")
	}
	if window.After != "2024-02-01T12:00:00Z" {
		t.Fatalf("unexpected after value: %q", window.After)
	}
}

func TestAnalyzeRejectsClientMatchers(t *testing.T) {
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

	rootCmd.SetArgs([]string{"analyze", "--config", path})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected analyze to fail with client matchers")
	}
	if !strings.Contains(err.Error(), "client matchers") {
		t.Fatalf("expected client matchers error, got: %v", err)
	}
}
