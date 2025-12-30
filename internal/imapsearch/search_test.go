package imapsearch

import (
	"bytes"
	"context"
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

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

func TestSearchByMatchersLocalServer(t *testing.T) {
	tlsConfig := testTLSConfig(t)
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("user@example.com", "password")
	mem.AddUser(user)

	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create mailbox: %v", err)
	}

	if _, err := user.Append("INBOX", newLiteral(t, sampleMessage(
		"News <news@example.com>",
		"User <user@example.com>",
		"Hello",
		"Please unsubscribe from these updates.",
	)), &imap.AppendOptions{}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	if _, err := user.Append("INBOX", newLiteral(t, sampleMessage(
		"Other <other@example.org>",
		"User <user@example.com>",
		"Hi",
		"Nothing to see here.",
	)), &imap.AppendOptions{}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	server := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		TLSConfig:    tlsConfig,
		InsecureAuth: true,
	})

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		server.Close()
		ln.Close()
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	client := &Client{
		Addr:      ln.Addr().String(),
		Username:  "user@example.com",
		Password:  "password",
		Mailbox:   "INBOX",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	cases := []struct {
		name     string
		matchers config.Matchers
		want     int
	}{
		{
			name: "match sender recipient body",
			matchers: config.Matchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe"},
			},
			want: 1,
		},
		{
			name: "match sender email",
			matchers: config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			},
			want: 1,
		},
		{
			name: "match recipients email",
			matchers: config.Matchers{
				Folders:    []string{"INBOX"},
				Recipients: []string{"user@example.com"},
			},
			want: 2,
		},
		{
			name: "no matches",
			matchers: config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"nope"},
				Recipients:      []string{"user@example.com"},
				BodySubstring:   []string{"unsubscribe"},
			},
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			uids, err := client.SearchByMatchers(ctx, tc.matchers)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(uids) != tc.want {
				t.Fatalf("expected %d match(es), got %d", tc.want, len(uids))
			}
		})
	}

	_ = server.Close()
	_ = ln.Close()

	select {
	case <-errCh:
	default:
	}
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

func sampleMessage(from, to, subject, body string) string {
	builder := &strings.Builder{}
	builder.WriteString("From: ")
	builder.WriteString(from)
	builder.WriteString("\r\n")
	builder.WriteString("To: ")
	builder.WriteString(to)
	builder.WriteString("\r\n")
	builder.WriteString("Subject: ")
	builder.WriteString(subject)
	builder.WriteString("\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(body)
	builder.WriteString("\r\n")
	return builder.String()
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
