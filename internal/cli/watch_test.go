package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/emersion/go-imap/v2"
	giimapserver "github.com/emersion/go-imap/v2/imapserver"
	giimapmemserver "github.com/emersion/go-imap/v2/imapserver/imapmemserver"
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

func TestWatchTestHappyPath(t *testing.T) {
	addr, cleanup := setupWatchTestServer(t, defaultMailbox, sampleWatchTestMessage(
		"News <news@example.com>",
		"User <user@example.com>",
		"ROM list",
		"f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	))
	t.Cleanup(cleanup)

	host, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	t.Setenv("POSTMANPAT_IMAP_HOST", host)
	t.Setenv("POSTMANPAT_IMAP_PORT", port)
	t.Setenv("POSTMANPAT_IMAP_USER", "user@example.com")
	t.Setenv("POSTMANPAT_IMAP_PASS", "password")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
rules:
  - name: "ROM"
    client:
      list_id_regex:
        - '<f7443300a7bb349db1e85fa6e\.1520313\.list-id\.mcsv\.net>'
    actions: []
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.SetArgs([]string{"watch", "--test", "ROM", "--config", path, "--limit", "10"})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("watch test failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "test match") {
		t.Fatalf("expected test match output, got: %s", out)
	}
	if !strings.Contains(out, "watch test complete") || !strings.Contains(out, "matches=1") {
		t.Fatalf("expected watch test completion with matches=1, got: %s", out)
	}
}

func TestWatchTestNoMatches(t *testing.T) {
	addr, cleanup := setupWatchTestServer(t, defaultMailbox, sampleWatchTestMessage(
		"News <news@example.com>",
		"User <user@example.com>",
		"ROM list",
		"f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	))
	t.Cleanup(cleanup)

	host, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	t.Setenv("POSTMANPAT_IMAP_HOST", host)
	t.Setenv("POSTMANPAT_IMAP_PORT", port)
	t.Setenv("POSTMANPAT_IMAP_USER", "user@example.com")
	t.Setenv("POSTMANPAT_IMAP_PASS", "password")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
rules:
  - name: "ROM"
    client:
      list_id_regex:
        - "<not-a-match>"
    actions: []
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.SetArgs([]string{"watch", "--test", "ROM", "--config", path, "--limit", "10"})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("watch test failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "watch test complete") || !strings.Contains(out, "matches=0") {
		t.Fatalf("expected watch test completion with matches=0, got: %s", out)
	}
}

func TestWatchTestMailboxOverride(t *testing.T) {
	addr, cleanup := setupWatchTestServer(t, "Archive", sampleWatchTestMessage(
		"News <news@example.com>",
		"User <user@example.com>",
		"ROM list",
		"f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	))
	t.Cleanup(cleanup)

	host, port, err := splitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	t.Setenv("POSTMANPAT_IMAP_HOST", host)
	t.Setenv("POSTMANPAT_IMAP_PORT", port)
	t.Setenv("POSTMANPAT_IMAP_USER", "user@example.com")
	t.Setenv("POSTMANPAT_IMAP_PASS", "password")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
rules:
  - name: "ROM"
    client:
      list_id_regex:
        - '<f7443300a7bb349db1e85fa6e\.1520313\.list-id\.mcsv\.net>'
    actions: []
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.SetArgs([]string{"watch", "--test", "ROM", "--config", path, "--limit", "10", "--mailbox", "Archive"})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("watch test failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "test match") {
		t.Fatalf("expected test match output, got: %s", out)
	}
	if !strings.Contains(out, "watch test complete") || !strings.Contains(out, "matches=1") {
		t.Fatalf("expected watch test completion with matches=1, got: %s", out)
	}
}

func setupWatchTestServer(t *testing.T, mailbox string, raw string) (string, func()) {
	t.Helper()

	tlsConfig, rootCAs := testWatchTLSConfig(t)
	mem := giimapmemserver.New()
	user := giimapmemserver.NewUser("user@example.com", "password")
	mem.AddUser(user)

	if err := user.Create(mailbox, nil); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}

	if _, err := user.Append(mailbox, newWatchLiteral(t, raw), &imap.AppendOptions{Time: time.Now()}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	server := giimapserver.New(&giimapserver.Options{
		NewSession: func(*giimapserver.Conn) (giimapserver.Session, *giimapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		TLSConfig:    tlsConfig,
		InsecureAuth: true,
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	watchTLSConfigProvider = func() *tls.Config {
		return &tls.Config{
			RootCAs:    rootCAs,
			ServerName: "localhost",
		}
	}

	cleanup := func() {
		_ = server.Close()
		_ = ln.Close()
		watchTLSConfigProvider = nil
		select {
		case <-errCh:
		default:
		}
	}

	return ln.Addr().String(), cleanup
}

type watchLiteralReader struct {
	*bytes.Reader
	size int64
}

func newWatchLiteral(t *testing.T, raw string) imap.LiteralReader {
	t.Helper()
	buf := []byte(raw)
	return &watchLiteralReader{
		Reader: bytes.NewReader(buf),
		size:   int64(len(buf)),
	}
}

func (lr *watchLiteralReader) Size() int64 {
	return lr.size
}

func sampleWatchTestMessage(from, to, subject, listID string) string {
	builder := &strings.Builder{}
	builder.WriteString("From: ")
	builder.WriteString(from)
	builder.WriteString("\r\n")
	builder.WriteString("To: ")
	builder.WriteString(to)
	builder.WriteString("\r\n")
	if strings.TrimSpace(listID) != "" {
		builder.WriteString("List-ID: ")
		builder.WriteString(listID)
		builder.WriteString("\r\n")
	}
	builder.WriteString("Subject: ")
	builder.WriteString(subject)
	builder.WriteString("\r\n")
	builder.WriteString("\r\n")
	builder.WriteString("Body")
	builder.WriteString("\r\n")
	return builder.String()
}

func testWatchTLSConfig(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	caSerial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber: caSerial,
		Subject: pkix.Name{
			CommonName: "postmanpat-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}

	serverSerial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate server serial: %v", err)
	}

	serverTemplate := x509.Certificate{
		SerialNumber: serverSerial,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{serverDER, caDER},
		PrivateKey:  serverKey,
	}

	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, rootPool
}

func splitHostPort(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}
	return host, port, nil
}
