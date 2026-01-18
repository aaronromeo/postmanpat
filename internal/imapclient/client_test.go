package imapclient

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
	"github.com/stretchr/testify/assert"
)

func TestSearchByMatchersLocalServer(t *testing.T) {
	client, ids, cleanup := setupTestServer(t, nil, nil, nil)
	t.Cleanup(cleanup)

	cases := []struct {
		name     string
		matchers config.Matchers
		wantUIDs []uint32
	}{
		{
			name: "match body substrings require all",
			matchers: config.Matchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe", "updates"},
			},
			wantUIDs: []uint32{ids.newsUID},
		},
		{
			name: "body substrings fail when missing",
			matchers: config.Matchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe", "missing"},
			},
			wantUIDs: nil,
		},
		{
			name: "match reply-to domain",
			matchers: config.Matchers{
				Folders:          []string{"INBOX"},
				ReplyToSubstring: []string{"example.com"},
			},
			wantUIDs: []uint32{ids.newsUID},
		},
		{
			name: "match age days",
			matchers: func() config.Matchers {
				age := 1
				return config.Matchers{
					Folders: []string{"INBOX"},
					AgeDays: &age,
				}
			}(),
			wantUIDs: []uint32{ids.newsUID},
		},
		{
			name: "match sender recipient body",
			matchers: config.Matchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe"},
			},
			wantUIDs: []uint32{ids.newsUID},
		},
		{
			name: "match sender email",
			matchers: config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			},
			wantUIDs: []uint32{ids.newsUID},
		},
		{
			name: "match recipients email",
			matchers: config.Matchers{
				Folders:    []string{"INBOX"},
				Recipients: []string{"user@example.com"},
			},
			wantUIDs: []uint32{ids.newsUID, ids.otherUID},
		},
		{
			name: "no matches",
			matchers: config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"nope"},
				Recipients:      []string{"user@example.com"},
				BodySubstring:   []string{"unsubscribe"},
			},
			wantUIDs: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)

			matched, err := client.SearchByMatchers(ctx, tc.matchers)
			assert.NoError(t, err, "search error")
			assert.ElementsMatch(t, tc.wantUIDs, matched["INBOX"], "unexpected UID set")
		})
	}

}

func TestDeleteUIDsLocalServer(t *testing.T) {
	cases := []struct {
		name string
		caps imap.CapSet
	}{
		{
			name: "uidplus",
			caps: imap.CapSet{
				imap.CapIMAP4rev1: {},
				imap.CapUIDPlus:   {},
			},
		},
		{
			name: "expunge",
			caps: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, ids, cleanup := setupTestServer(t, tc.caps, nil, nil)
			t.Cleanup(cleanup)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)

			target := config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			}
			matched, err := client.SearchByMatchers(ctx, target)
			assert.NoError(t, err, "search error")
			assert.ElementsMatch(t, []uint32{ids.newsUID}, matched["INBOX"], "unexpected matches before delete")

			err = client.DeleteByMailbox(ctx, matched)
			assert.NoError(t, err, "delete error")

			matched, err = client.SearchByMatchers(ctx, target)
			assert.NoError(t, err, "search after delete")
			assert.Empty(t, matched["INBOX"], "expected no matches after delete")

			remaining := config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.org"},
			}
			matched, err = client.SearchByMatchers(ctx, remaining)
			assert.NoError(t, err, "search remaining")
			assert.ElementsMatch(t, []uint32{ids.otherUID}, matched["INBOX"], "unexpected remaining matches")
		})
	}
}

func TestMoveByMailboxLocalServer(t *testing.T) {
	cases := []struct {
		name            string
		caps            imap.CapSet
		destination     string
		extraMailboxes  []string
		expectError     bool
		expectInArchive bool
	}{
		{
			name:        "move-capability",
			destination: "Archive",
			extraMailboxes: []string{
				"Archive",
			},
			caps: imap.CapSet{
				imap.CapIMAP4rev1: {},
				imap.CapMove:      {},
			},
			expectInArchive: true,
		},
		{
			name:        "move-fallback",
			destination: "Archive",
			extraMailboxes: []string{
				"Archive",
			},
			caps:            nil,
			expectInArchive: true,
		},
		{
			name:        "missing-destination",
			destination: "DoesNotExist",
			caps:        nil,
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, ids, cleanup := setupTestServer(t, tc.caps, tc.extraMailboxes, nil)
			t.Cleanup(cleanup)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)

			_, err := client.SearchByMatchers(ctx, config.Matchers{
				Folders: []string{"INBOX"},
			})
			assert.NoError(t, err, "select inbox before move")

			err = client.MoveByMailbox(ctx, map[string][]uint32{"INBOX": []uint32{ids.newsUID}}, tc.destination)
			if tc.expectError {
				assert.Error(t, err, "expected move error")
				return
			}
			assert.NoError(t, err, "move error")

			inboxMatchers := config.Matchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			}
			matched, err := client.SearchByMatchers(ctx, inboxMatchers)
			assert.NoError(t, err, "search inbox after move")
			assert.Empty(t, matched["INBOX"], "expected no matches in INBOX after move")

			if tc.expectInArchive {
				archiveClient := &Client{
					Addr:      client.Addr,
					Username:  client.Username,
					Password:  client.Password,
					TLSConfig: client.TLSConfig,
				}
				err = archiveClient.Connect()
				assert.NoError(t, err, "connect archive client")
				if err == nil {
					t.Cleanup(func() {
						_ = archiveClient.Close()
					})
					archiveMatchers := config.Matchers{
						Folders:         []string{tc.destination},
						SenderSubstring: []string{"example.com"},
					}
					matched, err = archiveClient.SearchByMatchers(ctx, archiveMatchers)
					assert.NoError(t, err, "search archive after move")
					assert.Len(t, matched[tc.destination], 1, "expected moved message in destination")
				}
			}
		})
	}
}

func TestSearchByMatchersMultipleFolders(t *testing.T) {
	extraMessages := []testMailboxMessage{
		{
			Mailbox: "Archive",
			From:    "Archive <archive@example.net>",
			To:      "User <user@example.com>",
			ReplyTo: "Reply <reply@example.net>",
			Subject: "Archive notice",
			Body:    "Stored in archive.",
		},
	}
	client, ids, cleanup := setupTestServer(t, nil, []string{"Archive"}, extraMessages)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	matchers := config.Matchers{
		Folders:         []string{"INBOX", "Archive"},
		SenderSubstring: []string{"example."},
	}
	matched, err := client.SearchByMatchers(ctx, matchers)
	assert.NoError(t, err, "search error")
	assert.ElementsMatch(t, []uint32{ids.newsUID, ids.otherUID}, matched["INBOX"], "unexpected INBOX matches")
	assert.Len(t, matched["Archive"], 1, "expected one Archive match")
}

func TestClientReuseAcrossOperations(t *testing.T) {
	client, ids, cleanup := setupTestServer(t, nil, []string{"Archive"}, nil)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	matched, err := client.SearchByMatchers(ctx, config.Matchers{
		Folders:         []string{"INBOX"},
		SenderSubstring: []string{"example.com"},
	})
	assert.NoError(t, err, "search initial sender")
	assert.ElementsMatch(t, []uint32{ids.newsUID}, matched["INBOX"], "unexpected initial sender matches")

	err = client.DeleteUIDs(ctx, matched["INBOX"])
	assert.NoError(t, err, "delete sender matches")

	matched, err = client.SearchByMatchers(ctx, config.Matchers{
		Folders:         []string{"INBOX"},
		SenderSubstring: []string{"example.com"},
	})
	assert.NoError(t, err, "search after delete")
	assert.Empty(t, matched["INBOX"], "expected no matches after delete")

	err = client.MoveUIDs(ctx, []uint32{ids.otherUID}, "Archive")
	assert.NoError(t, err, "move other message")

	matched, err = client.SearchByMatchers(ctx, config.Matchers{
		Folders:         []string{"INBOX"},
		SenderSubstring: []string{"example.org"},
	})
	assert.NoError(t, err, "search inbox after move")
	assert.Empty(t, matched["INBOX"], "expected no matches in INBOX after move")
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

type testMessageIDs struct {
	newsUID  uint32
	otherUID uint32
}

type testMailboxMessage struct {
	Mailbox string
	From    string
	To      string
	ReplyTo string
	Subject string
	Body    string
	Time    time.Time
}

func setupTestServer(t *testing.T, caps imap.CapSet, extraMailboxes []string, extraMessages []testMailboxMessage) (*Client, testMessageIDs, func()) {
	t.Helper()

	tlsConfig := testTLSConfig(t)
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("user@example.com", "password")
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

	server := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
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

	client := &Client{
		Addr:      ln.Addr().String(),
		Username:  "user@example.com",
		Password:  "password",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		_ = ln.Close()
		_ = server.Close()
		t.Fatalf("connect: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		_ = server.Close()
		_ = ln.Close()
		select {
		case <-errCh:
		default:
		}
	}

	ids := testMessageIDs{
		newsUID:  uint32(newsAppend.UID),
		otherUID: uint32(otherAppend.UID),
	}

	return client, ids, cleanup
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

func TestBuildSearchCriteriaListIDSubstring(t *testing.T) {
	matchers := config.Matchers{
		ListIDSubstring: []string{"list.example.com"},
	}

	criteria := buildSearchCriteria(matchers)
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.Header) != 1 {
		t.Fatalf("expected 1 header criteria, got %d", len(criteria.Header))
	}
	if criteria.Header[0].Key != "List-ID" {
		t.Fatalf("expected List-ID header key, got %q", criteria.Header[0].Key)
	}
	if criteria.Header[0].Value != "list.example.com" {
		t.Fatalf("expected List-ID header value, got %q", criteria.Header[0].Value)
	}
}

func TestBuildSearchCriteriaListIDSubstringSkipsEmpty(t *testing.T) {
	matchers := config.Matchers{
		ListIDSubstring: []string{"", "   "},
	}

	criteria := buildSearchCriteria(matchers)
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.Header) != 0 {
		t.Fatalf("expected no header criteria, got %d", len(criteria.Header))
	}
}

func TestBuildSearchCriteriaExcludesDeleted(t *testing.T) {
	criteria := buildSearchCriteria(config.Matchers{})
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.NotFlag) == 0 {
		t.Fatal("expected NotFlag to include \\Deleted")
	}
	found := false
	for _, flag := range criteria.NotFlag {
		if flag == imap.FlagDeleted {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected NotFlag to include \\Deleted")
	}
}
