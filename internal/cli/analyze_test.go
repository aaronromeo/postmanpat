package cli

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/internal/imapclient"
)

func TestBuildAnalyzeReportJSON(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	ageDays := 2
	data := []imapclient.MailData{
		{
			SenderDomains:          []string{"example.com"},
			ReplyToDomains:         []string{"reply.example.com"},
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

	report := buildAnalyzeReport(data, analyzeReportParams{
		Mailbox:   "INBOX",
		Account:   "user@example.com",
		Generated: now,
		AgeDays:   &ageDays,
		Options: analyzeOptions{
			Top:      100,
			Examples: 20,
			MinCount: 1,
		},
	})

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
	age := 0

	window := buildTimeWindow(now, &age)
	if window.Before == "" {
		t.Fatal("expected before timestamp")
	}
	if window.After == "" {
		t.Fatal("expected after timestamp with age_days set")
	}

	window = buildTimeWindow(now, nil)
	if window.Before == "" {
		t.Fatal("expected before timestamp")
	}
	if window.After != "" {
		t.Fatalf("expected empty after when age_days is nil, got %q", window.After)
	}
}
