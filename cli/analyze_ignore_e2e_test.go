package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/ftest"
)

const analyzeIgnoreTestConfig = `
rules:
  - name: "Analyze"
    server:
      age_window:
        min: "1h"
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
  cleanup:
    list_ids:
      - "announce.github"
`

func analyzeIgnoreTestMessages() []ftest.AnalyzeMessage {
	day := time.Now().Add(-24 * time.Hour)
	return []ftest.AnalyzeMessage{
		// Fully Decided: watch via domain github.com, cleanup via list-id -> filtered.
		{From: "news@github.com", To: "user@example.com", Subject: "GitHub Security Alert", ListID: "<announce.github.com>", Body: "alert", Time: day},
		// Watch-only (domain matches, list-id does not) -> retained.
		{From: "noreply@github.com", To: "user@example.com", Subject: "Pull Request Review", Body: "review", Time: day},
		// Unmatched -> retained.
		{From: "info@example.com", To: "user@example.com", Subject: "Weekly Newsletter", ListID: "<newsletter.example.com>", Body: "hi", Time: day},
	}
}

func runAnalyzeToReport(t *testing.T, extraArgs ...string) map[string]any {
	t.Helper()

	addr, cleanupServer := ftest.SetupAnalyzeIMAPServer(t, analyzeIgnoreTestMessages())
	t.Cleanup(cleanupServer)

	host, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	t.Setenv("POSTMANPAT_IMAP_HOST", host)
	t.Setenv("POSTMANPAT_IMAP_PORT", port)
	t.Setenv("POSTMANPAT_IMAP_USER", ftest.DefaultUser)
	t.Setenv("POSTMANPAT_IMAP_PASS", ftest.DefaultPass)

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(analyzeIgnoreTestConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Reset analyze flags to defaults to avoid cross-test pollution.
	_ = analyzeCmd.Flags().Set("no-ignore", "false")
	_ = analyzeCmd.Flags().Set("min-count", "2")
	_ = analyzeCmd.Flags().Set("top", "100")
	_ = analyzeCmd.Flags().Set("examples", "20")

	var output bytes.Buffer
	args := append([]string{"analyze", "--config", cfgPath}, extraArgs...)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	analyzeTLSConfigProvider = func() *tls.Config {
		return &tls.Config{InsecureSkipVerify: true}
	}
	t.Cleanup(func() { analyzeTLSConfigProvider = nil })

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("analyze failed: %v\noutput: %s", err, output.String())
	}

	reportPath := strings.TrimSpace(output.String())
	payload, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report %q: %v", reportPath, err)
	}

	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	return report
}

func analyzeTotalScanned(t *testing.T, report map[string]any) float64 {
	t.Helper()
	stats, ok := report["stats"].(map[string]any)
	if !ok {
		t.Fatal("report missing stats")
	}
	count, ok := stats["total_messages_scanned"].(float64)
	if !ok {
		t.Fatal("report missing total_messages_scanned")
	}
	return count
}

func TestAnalyzeFiltersFullyDecidedMessages(t *testing.T) {
	report := runAnalyzeToReport(t)

	if got := analyzeTotalScanned(t, report); got != 2 {
		t.Fatalf("expected 2 un-ignored messages, got %v", got)
	}

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if strings.Contains(string(raw), "ignored") {
		t.Fatal("report must stay silent about ignored messages (no ignored fields)")
	}
}

func TestAnalyzeNoIgnoreBypassesFiltering(t *testing.T) {
	report := runAnalyzeToReport(t, "--no-ignore")

	if got := analyzeTotalScanned(t, report); got != 3 {
		t.Fatalf("expected all 3 messages with --no-ignore, got %v", got)
	}
}

func TestAnalyzeAnnotatesWatchSuppressedCluster(t *testing.T) {
	report := runAnalyzeToReport(t, "--min-count", "1")

	indexes, ok := report["indexes"].(map[string]any)
	if !ok {
		t.Fatal("report missing indexes")
	}
	senderLens, ok := indexes["sender_unsub_lens"].(map[string]any)
	if !ok {
		t.Fatal("report missing sender_unsub_lens")
	}
	clusters, ok := senderLens["clusters"].([]any)
	if !ok {
		t.Fatal("sender_unsub_lens.clusters is invalid")
	}

	var found bool
	for _, c := range clusters {
		cluster := c.(map[string]any)
		keys := cluster["keys"].(map[string]any)
		domains, ok := keys["SenderDomains"].([]any)
		if ok && len(domains) == 1 && domains[0] == "github.com" {
			suppressed, ok := cluster["suppressed"].([]any)
			if !ok || len(suppressed) != 1 || suppressed[0] != "watch" {
				t.Fatalf("expected suppress=[watch], got %v", cluster["suppressed"])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected to find github.com cluster in sender_unsub_lens")
	}
}

func TestAnalyzeNoIgnoreNoSuppressedField(t *testing.T) {
	report := runAnalyzeToReport(t, "--no-ignore")

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "suppressed") {
		t.Fatal("report with --no-ignore must not contain 'suppressed' field")
	}
}
