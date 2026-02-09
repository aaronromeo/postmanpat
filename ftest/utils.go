package ftest

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	giimapserver "github.com/emersion/go-imap/v2/imapserver"
	giimapmemserver "github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

const (
	DefaultUser = "user@example.com"
	DefaultPass = "password"
)

type MessageIDs struct {
	NewsUID  uint32
	OtherUID uint32
}

type MailboxMessage struct {
	Mailbox string
	From    string
	To      string
	ReplyTo string
	Subject string
	Body    string
	Time    time.Time
}

type AnalyzeMessage struct {
	From    string
	To      string
	Subject string
	ListID  string
	Body    string
	Time    time.Time
}

type RawMessage struct {
	Mailbox string
	Raw     string
	Time    time.Time
}

func SetupIMAPServer(t *testing.T, caps imap.CapSet, extraMailboxes []string, extraMessages []MailboxMessage) (string, MessageIDs, func()) {
	t.Helper()

	tlsConfig := testTLSConfig(t)
	mem := giimapmemserver.New()
	user := giimapmemserver.NewUser(DefaultUser, DefaultPass)
	mem.AddUser(user)

	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}
	for _, mailbox := range extraMailboxes {
		if strings.TrimSpace(mailbox) == "" {
			continue
		}
		if err := user.Create(mailbox, nil); err != nil {
			t.Fatalf("create mailbox %q: %v", mailbox, err)
		}
	}

	newsAppend, err := user.Append("INBOX", newLiteral(t, sampleMessageWithReplyTo(
		"News <news@example.com>",
		"User <user@example.com>",
		"Reply <reply@example.com>",
		"Hello",
		"Please unsubscribe from these updates.",
	)), &imap.AppendOptions{Time: time.Now().Add(-48 * time.Hour)})
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	otherAppend, err := user.Append("INBOX", newLiteral(t, sampleMessageWithReplyTo(
		"Other <other@example.org>",
		"User <user@example.com>",
		"Reply <reply@example.org>",
		"Hi",
		"Nothing to see here.",
	)), &imap.AppendOptions{Time: time.Now()})
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	for _, msg := range extraMessages {
		mailbox := strings.TrimSpace(msg.Mailbox)
		if mailbox == "" {
			t.Fatalf("extra message mailbox is required")
		}
		appendTime := msg.Time
		if appendTime.IsZero() {
			appendTime = time.Now()
		}
		if _, err := user.Append(mailbox, newLiteral(t, sampleMessageWithReplyTo(
			msg.From,
			msg.To,
			msg.ReplyTo,
			msg.Subject,
			msg.Body,
		)), &imap.AppendOptions{Time: appendTime}); err != nil {
			t.Fatalf("append extra message: %v", err)
		}
	}

	server := giimapserver.New(&giimapserver.Options{
		NewSession: func(*giimapserver.Conn) (giimapserver.Session, *giimapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         caps,
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

	cleanup := func() {
		_ = server.Close()
		_ = ln.Close()
		select {
		case <-errCh:
		default:
		}
	}

	ids := MessageIDs{
		NewsUID:  uint32(newsAppend.UID),
		OtherUID: uint32(otherAppend.UID),
	}

	return ln.Addr().String(), ids, cleanup
}

func SetupAnalyzeIMAPServer(t *testing.T, messages []AnalyzeMessage) (string, func()) {
	t.Helper()

	tlsConfig := testAnalyzeTLSConfig(t)
	mem := giimapmemserver.New()
	user := giimapmemserver.NewUser(DefaultUser, DefaultPass)
	mem.AddUser(user)

	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}

	for _, msg := range messages {
		appendTime := msg.Time
		if appendTime.IsZero() {
			appendTime = time.Now()
		}
		if _, err := user.Append("INBOX", newAnalyzeLiteral(t, sampleAnalyzeMessage(
			msg.From,
			msg.To,
			msg.Subject,
			msg.ListID,
			msg.Body,
		)), &imap.AppendOptions{Time: appendTime}); err != nil {
			t.Fatalf("append message: %v", err)
		}
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

	cleanup := func() {
		_ = server.Close()
		_ = ln.Close()
		select {
		case <-errCh:
		default:
		}
	}

	return ln.Addr().String(), cleanup
}

func SetupRawIMAPServer(t *testing.T, caps imap.CapSet, extraMailboxes []string, messages []RawMessage) (string, func()) {
	t.Helper()

	tlsConfig := testTLSConfig(t)
	mem := giimapmemserver.New()
	user := giimapmemserver.NewUser(DefaultUser, DefaultPass)
	mem.AddUser(user)

	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}
	for _, mailbox := range extraMailboxes {
		if strings.TrimSpace(mailbox) == "" {
			continue
		}
		if err := user.Create(mailbox, nil); err != nil {
			t.Fatalf("create mailbox %q: %v", mailbox, err)
		}
	}

	for _, msg := range messages {
		mailbox := strings.TrimSpace(msg.Mailbox)
		if mailbox == "" {
			mailbox = "INBOX"
		}
		appendTime := msg.Time
		if appendTime.IsZero() {
			appendTime = time.Now()
		}
		if _, err := user.Append(mailbox, newLiteral(t, msg.Raw), &imap.AppendOptions{Time: appendTime}); err != nil {
			t.Fatalf("append raw message: %v", err)
		}
	}

	server := giimapserver.New(&giimapserver.Options{
		NewSession: func(*giimapserver.Conn) (giimapserver.Session, *giimapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         caps,
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

	cleanup := func() {
		_ = server.Close()
		_ = ln.Close()
		select {
		case <-errCh:
		default:
		}
	}

	return ln.Addr().String(), cleanup
}

type literalReader struct {
	*bytes.Reader
	size int64
}

func newLiteral(t *testing.T, raw string) imap.LiteralReader {
	t.Helper()
	buf := []byte(raw)
	return &literalReader{
		Reader: bytes.NewReader(buf),
		size:   int64(len(buf)),
	}
}

func (lr *literalReader) Size() int64 {
	return lr.size
}

func sampleMessageWithReplyTo(from, to, replyTo, subject, body string) string {
	builder := &strings.Builder{}
	builder.WriteString("From: ")
	builder.WriteString(from)
	builder.WriteString("\r\n")
	builder.WriteString("To: ")
	builder.WriteString(to)
	builder.WriteString("\r\n")
	builder.WriteString("Reply-To: ")
	builder.WriteString(replyTo)
	builder.WriteString("\r\n")
	builder.WriteString("Subject: ")
	builder.WriteString(subject)
	builder.WriteString("\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(body)
	builder.WriteString("\r\n")
	return builder.String()
}

func sampleAnalyzeMessage(from, to, subject, listID, body string) string {
	builder := &strings.Builder{}
	builder.WriteString("From: ")
	builder.WriteString(from)
	builder.WriteString("\r\n")
	builder.WriteString("To: ")
	builder.WriteString(to)
	builder.WriteString("\r\n")
	if listID != "" {
		builder.WriteString("List-ID: ")
		builder.WriteString(listID)
		builder.WriteString("\r\n")
	}
	builder.WriteString("Subject: ")
	builder.WriteString(subject)
	builder.WriteString("\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(body)
	builder.WriteString("\r\n")
	return builder.String()
}

func newAnalyzeLiteral(t *testing.T, raw string) imap.LiteralReader {
	return newLiteral(t, raw)
}

func testTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
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

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"imap"},
	}
}

func testAnalyzeTLSConfig(t *testing.T) *tls.Config {
	return testTLSConfig(t)
}
