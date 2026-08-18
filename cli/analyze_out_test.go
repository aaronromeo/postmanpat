package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronromeo/postmanpat/ftest"
)

const analyzeOutTestConfig = `
rules:
  - name: "Nightly INBOX"
    server:
      folders:
        - "INBOX"
    actions: []
  - name: "Archive Scan"
    server:
      folders:
        - "INBOX"
    actions: []
`

const analyzeOutCollisionConfig = `
rules:
  - name: "INBOX Scan!"
    server:
      folders:
        - "INBOX"
    actions: []
  - name: "INBOX Scan?"
    server:
      folders:
        - "INBOX"
    actions: []
`

// runAnalyzeOut sets up an in-memory IMAP server, writes the given config to a
// temp file, and runs `analyze` with the provided extra args. It returns the
// captured stdout so callers can assert on the printed report paths.
func runAnalyzeOut(t *testing.T, cfg string, extraArgs ...string) *bytes.Buffer {
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
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Reset analyze flags to defaults to avoid cross-test pollution.
	_ = analyzeCmd.Flags().Set("out", "")
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
	return &output
}

func TestAnalyzeOutWritesDeterministicPerRuleFiles(t *testing.T) {
	outDir := t.TempDir()

	output := runAnalyzeOut(t, analyzeOutTestConfig, "--out", outDir, "--min-count", "1")

	wantFiles := map[string]bool{
		"postmanpat-analyze-nightly-inbox.json": true,
		"postmanpat-analyze-archive-scan.json":  true,
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = true
	}
	for name := range wantFiles {
		if !got[name] {
			t.Errorf("expected report file %q in %s; got entries %v", name, outDir, got)
		}
		if !strings.Contains(output.String(), filepath.Join(outDir, name)) {
			t.Errorf("expected stdout to contain path %q; got %q", filepath.Join(outDir, name), output.String())
		}
		full := filepath.Join(outDir, name)
		payload, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read %s: %v", full, err)
		}
		var report map[string]any
		if err := json.Unmarshal(payload, &report); err != nil {
			t.Fatalf("parse %s as JSON: %v", full, err)
		}
		stats, ok := report["stats"].(map[string]any)
		if !ok {
			t.Fatalf("%s: report missing stats object", full)
		}
		if stats["total_messages_scanned"] == nil {
			t.Fatalf("%s: stats missing total_messages_scanned", full)
		}
	}
	if len(got) != len(wantFiles) {
		t.Errorf("expected exactly %d files in out dir, got %d (%v)", len(wantFiles), len(got), got)
	}
}

func TestAnalyzeOutSlugCollisionFailsBeforeScan(t *testing.T) {
	outDir := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(analyzeOutCollisionConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// No IMAP server is started. Dummy unreachable env ensures that, if the
	// collision preflight did NOT run before connecting, the failure would be a
	// connection error rather than the slug error this test asserts.
	t.Setenv("POSTMANPAT_IMAP_HOST", "127.0.0.1")
	t.Setenv("POSTMANPAT_IMAP_PORT", "1")
	t.Setenv("POSTMANPAT_IMAP_USER", ftest.DefaultUser)
	t.Setenv("POSTMANPAT_IMAP_PASS", ftest.DefaultPass)

	_ = analyzeCmd.Flags().Set("min-count", "1")
	t.Cleanup(func() { _ = analyzeCmd.Flags().Set("out", "") })

	var output bytes.Buffer
	rootCmd.SetArgs([]string{"analyze", "--config", cfgPath, "--out", outDir})
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected analyze to fail on slug collision, got nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "INBOX Scan!") || !strings.Contains(msg, "INBOX Scan?") {
		t.Fatalf("expected error to name both colliding rules; got %q", msg)
	}
	if !strings.Contains(msg, "slug") {
		t.Fatalf("expected error to mention slug; got %q", msg)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files written before the collision error; got %v", entries)
	}
}

func TestAnalyzeOutOverwritesPreviousRun(t *testing.T) {
	outDir := t.TempDir()
	runAnalyzeOut(t, analyzeOutTestConfig, "--out", outDir, "--min-count", "1")

	target := filepath.Join(outDir, "postmanpat-analyze-nightly-inbox.json")
	if err := os.WriteFile(target, []byte("NOT VALID JSON"), 0o600); err != nil {
		t.Fatalf("corrupt %s: %v", target, err)
	}

	runAnalyzeOut(t, analyzeOutTestConfig, "--out", outDir, "--min-count", "1")

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected exactly 2 files after re-run (overwrite, not accumulate); got %d", len(entries))
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("expected overwritten file to be valid JSON; got parse error: %v", err)
	}
	if report["stats"] == nil {
		t.Fatal("overwritten report missing stats")
	}
}

func TestAnalyzeWithoutOutPreservesTempFile(t *testing.T) {
	output := runAnalyzeOut(t, analyzeIgnoreTestConfig, "--min-count", "1")

	reportPath := strings.TrimSpace(output.String())
	if !strings.Contains(reportPath, "postmanpat-analyze-") {
		t.Fatalf("expected temp-file path containing 'postmanpat-analyze-'; got %q", reportPath)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected printed temp path to exist: %v", err)
	}
	payload, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read %s: %v", reportPath, err)
	}
	var report map[string]any
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("parse %s: %v", reportPath, err)
	}
}

func TestAnalyzeOutCreatesMissingDirectory(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "does-not-exist", "deeper")

	runAnalyzeOut(t, analyzeIgnoreTestConfig, "--out", outDir, "--min-count", "1")

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("expected MkdirAll to create nested out dir %s: %v", outDir, err)
	}
	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "postmanpat-analyze-") && strings.HasSuffix(e.Name(), ".json") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a postmanpat-analyze-*.json report in created dir %s; got %v", outDir, entries)
	}
}
