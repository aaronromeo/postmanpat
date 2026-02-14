package imap

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/ftest"
	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/matchers"
	"github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/stretchr/testify/assert"
)

func TestSearchByMatchersLocalServer(t *testing.T) {
	client, ids, cleanup := setupTestServer(t, nil, nil, nil)
	t.Cleanup(cleanup)

	cases := []struct {
		name     string
		matchers config.ServerMatchers
		wantUIDs []uint32
	}{
		{
			name: "match body substrings require all",
			matchers: config.ServerMatchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe", "updates"},
			},
			wantUIDs: []uint32{ids.NewsUID},
		},
		{
			name: "body substrings fail when missing",
			matchers: config.ServerMatchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe", "missing"},
			},
			wantUIDs: nil,
		},
		{
			name: "match reply-to domain",
			matchers: config.ServerMatchers{
				Folders:          []string{"INBOX"},
				ReplyToSubstring: []string{"example.com"},
			},
			wantUIDs: []uint32{ids.NewsUID},
		},
		{
			name: "match age window max",
			matchers: config.ServerMatchers{
				Folders: []string{"INBOX"},
				AgeWindow: &config.AgeWindow{
					Max: "1d",
				},
			},
			wantUIDs: []uint32{ids.OtherUID},
		},
		{
			name: "match age window min",
			matchers: config.ServerMatchers{
				Folders: []string{"INBOX"},
				AgeWindow: &config.AgeWindow{
					Min: "1d",
				},
			},
			wantUIDs: []uint32{ids.NewsUID},
		},
		{
			name: "match sender recipient body",
			matchers: config.ServerMatchers{
				Folders:       []string{"INBOX"},
				BodySubstring: []string{"unsubscribe"},
			},
			wantUIDs: []uint32{ids.NewsUID},
		},
		{
			name: "match sender email",
			matchers: config.ServerMatchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			},
			wantUIDs: []uint32{ids.NewsUID},
		},
		{
			name: "match recipients email",
			matchers: config.ServerMatchers{
				Folders:    []string{"INBOX"},
				Recipients: []string{"user@example.com"},
			},
			wantUIDs: []uint32{ids.NewsUID, ids.OtherUID},
		},
		{
			name: "no matches",
			matchers: config.ServerMatchers{
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

			matched, err := client.SearchByServerMatchers(ctx, tc.matchers)
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

			target := config.ServerMatchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			}
			matched, err := client.SearchByServerMatchers(ctx, target)
			assert.NoError(t, err, "search error")
			assert.ElementsMatch(t, []uint32{ids.NewsUID}, matched["INBOX"], "unexpected matches before delete")

			err = client.DeleteByMailbox(ctx, matched, true)
			assert.NoError(t, err, "delete error")

			matched, err = client.SearchByServerMatchers(ctx, target)
			assert.NoError(t, err, "search after delete")
			assert.Empty(t, matched["INBOX"], "expected no matches after delete")

			remaining := config.ServerMatchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.org"},
			}
			matched, err = client.SearchByServerMatchers(ctx, remaining)
			assert.NoError(t, err, "search remaining")
			assert.ElementsMatch(t, []uint32{ids.OtherUID}, matched["INBOX"], "unexpected remaining matches")
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

			_, err := client.SearchByServerMatchers(ctx, config.ServerMatchers{
				Folders: []string{"INBOX"},
			})
			assert.NoError(t, err, "select inbox before move")

			err = client.MoveByMailbox(ctx, map[string][]uint32{"INBOX": []uint32{ids.NewsUID}}, tc.destination)
			if tc.expectError {
				assert.Error(t, err, "expected move error")
				return
			}
			assert.NoError(t, err, "move error")

			inboxMatchers := config.ServerMatchers{
				Folders:         []string{"INBOX"},
				SenderSubstring: []string{"example.com"},
			}
			matched, err := client.SearchByServerMatchers(ctx, inboxMatchers)
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
					archiveMatchers := config.ServerMatchers{
						Folders:         []string{tc.destination},
						SenderSubstring: []string{"example.com"},
					}
					matched, err = archiveClient.SearchByServerMatchers(ctx, archiveMatchers)
					assert.NoError(t, err, "search archive after move")
					assert.Len(t, matched[tc.destination], 1, "expected moved message in destination")
				}
			}
		})
	}
}

func TestSearchByMatchersMultipleFolders(t *testing.T) {
	extraMessages := []ftest.MailboxMessage{
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

	matchers := config.ServerMatchers{
		Folders:         []string{"INBOX", "Archive"},
		SenderSubstring: []string{"example."},
	}
	matched, err := client.SearchByServerMatchers(ctx, matchers)
	assert.NoError(t, err, "search error")
	assert.ElementsMatch(t, []uint32{ids.NewsUID, ids.OtherUID}, matched["INBOX"], "unexpected INBOX matches")
	assert.Len(t, matched["Archive"], 1, "expected one Archive match")
}

func TestClientReuseAcrossOperations(t *testing.T) {
	client, ids, cleanup := setupTestServer(t, nil, []string{"Archive"}, nil)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	matched, err := client.SearchByServerMatchers(ctx, config.ServerMatchers{
		Folders:         []string{"INBOX"},
		SenderSubstring: []string{"example.com"},
	})
	assert.NoError(t, err, "search initial sender")
	assert.ElementsMatch(t, []uint32{ids.NewsUID}, matched["INBOX"], "unexpected initial sender matches")

	err = client.DeleteUIDs(ctx, matched["INBOX"], true)
	assert.NoError(t, err, "delete sender matches")

	matched, err = client.SearchByServerMatchers(ctx, config.ServerMatchers{
		Folders:         []string{"INBOX"},
		SenderSubstring: []string{"example.com"},
	})
	assert.NoError(t, err, "search after delete")
	assert.Empty(t, matched["INBOX"], "expected no matches after delete")

	err = client.MoveUIDs(ctx, []uint32{ids.OtherUID}, "Archive")
	assert.NoError(t, err, "move other message")

	matched, err = client.SearchByServerMatchers(ctx, config.ServerMatchers{
		Folders:         []string{"INBOX"},
		SenderSubstring: []string{"example.org"},
	})
	assert.NoError(t, err, "search inbox after move")
	assert.Empty(t, matched["INBOX"], "expected no matches in INBOX after move")
}

func setupTestServer(t *testing.T, caps imap.CapSet, extraMailboxes []string, extraMessages []ftest.MailboxMessage) (*Client, ftest.MessageIDs, func()) {
	t.Helper()

	addr, ids, cleanup := ftest.SetupIMAPServer(t, caps, extraMailboxes, extraMessages)

	client := &Client{
		Addr:      addr,
		Username:  ftest.DefaultUser,
		Password:  ftest.DefaultPass,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		cleanup()
		t.Fatalf("connect: %v", err)
	}

	return client, ids, func() {
		_ = client.Close()
		cleanup()
	}
}

func TestBuildSearchCriteriaListIDSubstring(t *testing.T) {
	matchers := config.ServerMatchers{
		ListIDSubstring: []string{"list.example.com"},
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
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
	matchers := config.ServerMatchers{
		ListIDSubstring: []string{"", "   "},
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.Header) != 0 {
		t.Fatalf("expected no header criteria, got %d", len(criteria.Header))
	}
}

func TestBuildSearchCriteriaReturnPathSubstring(t *testing.T) {
	matchers := config.ServerMatchers{
		ReturnPathSubstring: []string{"srs.messagingengine.com"},
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.Header) != 1 {
		t.Fatalf("expected 1 header criteria, got %d", len(criteria.Header))
	}
	if criteria.Header[0].Key != "Return-Path" {
		t.Fatalf("expected Return-Path header key, got %q", criteria.Header[0].Key)
	}
	if criteria.Header[0].Value != "srs.messagingengine.com" {
		t.Fatalf("expected Return-Path header value, got %q", criteria.Header[0].Value)
	}
}

func TestBuildSearchCriteriaSeenTrue(t *testing.T) {
	seen := true
	matchers := config.ServerMatchers{
		Seen: &seen,
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	found := false
	for _, flag := range criteria.Flag {
		if flag == imap.FlagSeen {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected \\Seen flag to be included")
	}
}

func TestBuildSearchCriteriaListUnsubscribeTrue(t *testing.T) {
	listUnsub := true
	matchers := config.ServerMatchers{
		ListUnsubscribe: &listUnsub,
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.Header) != 1 {
		t.Fatalf("expected 1 header criteria, got %d", len(criteria.Header))
	}
	if criteria.Header[0].Key != "List-Unsubscribe" {
		t.Fatalf("expected List-Unsubscribe header key, got %q", criteria.Header[0].Key)
	}
}

func TestBuildSearchCriteriaListUnsubscribeFalse(t *testing.T) {
	listUnsub := false
	matchers := config.ServerMatchers{
		ListUnsubscribe: &listUnsub,
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	if len(criteria.Not) != 1 {
		t.Fatalf("expected 1 NOT criteria, got %d", len(criteria.Not))
	}
	if len(criteria.Not[0].Header) != 1 {
		t.Fatalf("expected 1 NOT header criteria, got %d", len(criteria.Not[0].Header))
	}
	if criteria.Not[0].Header[0].Key != "List-Unsubscribe" {
		t.Fatalf("expected List-Unsubscribe header key, got %q", criteria.Not[0].Header[0].Key)
	}
}

func TestBuildSearchCriteriaSeenFalse(t *testing.T) {
	seen := false
	matchers := config.ServerMatchers{
		Seen: &seen,
	}

	criteria, err := buildSearchCriteria(matchers)
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
	if criteria == nil {
		t.Fatal("expected criteria")
	}
	found := false
	for _, flag := range criteria.NotFlag {
		if flag == imap.FlagSeen {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected \\Seen to be included in NotFlag")
	}
}

func TestBuildSearchCriteriaExcludesDeleted(t *testing.T) {
	criteria, err := buildSearchCriteria(config.ServerMatchers{})
	if err != nil {
		t.Fatalf("build criteria: %v", err)
	}
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

func TestListIDRegexEndToEnd(t *testing.T) {
	addr, cleanup := ftest.SetupAnalyzeIMAPServer(t, []ftest.AnalyzeMessage{
		{
			From:    "News <news@example.com>",
			To:      "User <user@example.com>",
			Subject: "ROM list",
			ListID:  "f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
			Body:    "unsubscribe",
			Time:    time.Now().Add(-2 * time.Hour),
		},
	})
	t.Cleanup(cleanup)

	client := &Client{
		Addr:      addr,
		Username:  "user@example.com",
		Password:  "password",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if _, err := client.SelectMailbox(ctx, "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		t.Fatalf("search uids: %v", err)
	}
	if len(uids) != 1 {
		t.Fatalf("expected 1 uid, got %d", len(uids))
	}

	data, err := client.FetchSenderData(ctx, uids)
	if err != nil {
		t.Fatalf("fetch data: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 message, got %d", len(data))
	}

	ok, err := matchers.MatchesClient(&config.ClientMatchers{
		ListIDRegex: []string{`<f7443300a7bb349db1e85fa6e\.1520313\.list-id\.mcsv\.net>`},
	}, matchers.ClientMessage{ListID: data[0].ListID})
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected list_id_regex to match ListID")
	}
}

func TestFetchSenderDataReplyToRequiresHeader(t *testing.T) {
	addr, cleanup := ftest.SetupAnalyzeIMAPServer(t, []ftest.AnalyzeMessage{
		{
			From:    "BandsInTown <updates@bandsintown.com>",
			To:      "User <user@example.com>",
			Subject: "Next Week",
			ListID:  "",
			Body:    "unsubscribe",
			Time:    time.Now().Add(-2 * time.Hour),
		},
	})
	t.Cleanup(cleanup)

	client := &Client{
		Addr:      addr,
		Username:  "user@example.com",
		Password:  "password",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if _, err := client.SelectMailbox(ctx, "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		t.Fatalf("search uids: %v", err)
	}
	if len(uids) != 1 {
		t.Fatalf("expected 1 uid, got %d", len(uids))
	}

	data, err := client.FetchSenderData(ctx, uids)
	if err != nil {
		t.Fatalf("fetch data: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 message, got %d", len(data))
	}
	if len(data[0].ReplyToDomains) != 0 {
		t.Fatalf("expected no reply-to domains when header is missing, got %v", data[0].ReplyToDomains)
	}
}

func TestFetchSenderDataDoesNotSetSeen(t *testing.T) {
	addr, cleanup := ftest.SetupAnalyzeIMAPServer(t, []ftest.AnalyzeMessage{
		{
			From:    "News <news@example.com>",
			To:      "User <user@example.com>",
			Subject: "Peek test",
			ListID:  "list.example.com",
			Body:    "unsubscribe",
			Time:    time.Now().Add(-2 * time.Hour),
		},
	})
	t.Cleanup(cleanup)

	client := &Client{
		Addr:      addr,
		Username:  "user@example.com",
		Password:  "password",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if _, err := client.SelectMailbox(ctx, "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		t.Fatalf("search uids: %v", err)
	}
	if len(uids) != 1 {
		t.Fatalf("expected 1 uid, got %d", len(uids))
	}

	flagsBefore, err := fetchMessageFlags(ctx, client, uids)
	if err != nil {
		t.Fatalf("fetch flags before: %v", err)
	}
	if containsFlag(flagsBefore, imap.FlagSeen) {
		t.Fatalf("expected message to be unseen before fetch, got flags %v", flagsBefore)
	}

	_, err = client.FetchSenderData(ctx, uids)
	if err != nil {
		t.Fatalf("fetch data: %v", err)
	}

	flagsAfter, err := fetchMessageFlags(ctx, client, uids)
	if err != nil {
		t.Fatalf("fetch flags after: %v", err)
	}
	if containsFlag(flagsAfter, imap.FlagSeen) {
		t.Fatalf("expected message to remain unseen after fetch, got flags %v", flagsAfter)
	}
}

func TestFetchSenderDataMalformedHeaderDoesNotError(t *testing.T) {
	raw := "From: News <news@example.com>\r\n" +
		"To: User <user@example.com>\r\n" +
		"BadHeader\r\n" +
		"Subject: Hello\r\n" +
		"\r\n" +
		"Body\r\n"

	addr, cleanup := ftest.SetupRawIMAPServer(t, nil, nil, []ftest.RawMessage{{
		Mailbox: "INBOX",
		Raw:     raw,
		Time:    time.Now(),
	}})
	t.Cleanup(cleanup)

	client := &Client{
		Addr:      addr,
		Username:  ftest.DefaultUser,
		Password:  ftest.DefaultPass,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if _, err := client.SelectMailbox(ctx, "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		t.Fatalf("search uids: %v", err)
	}
	if len(uids) != 1 {
		t.Fatalf("expected 1 uid, got %d", len(uids))
	}

	data, err := client.FetchSenderData(ctx, uids)
	if err != nil {
		t.Fatalf("fetch data: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 message, got %d", len(data))
	}
	if data[0].ListID != "" {
		t.Fatalf("expected empty ListID for malformed header, got %q", data[0].ListID)
	}
}

func TestFetchSenderDataReturnPathDomain(t *testing.T) {
	addr, cleanup := ftest.SetupRawIMAPServer(t, nil, nil, []ftest.RawMessage{{
		Mailbox: "INBOX",
		Raw: "From: News <news@example.com>\r\n" +
			"To: User <user@example.com>\r\n" +
			"Return-Path: <SRS0=foo@SRS.MessagingEngine.com>\r\n" +
			"Subject: Hello\r\n" +
			"\r\n" +
			"Body\r\n",
		Time: time.Now(),
	}})
	t.Cleanup(cleanup)

	client := &Client{
		Addr:      addr,
		Username:  ftest.DefaultUser,
		Password:  ftest.DefaultPass,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx := context.Background()
	if _, err := client.SelectMailbox(ctx, "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		t.Fatalf("search uids: %v", err)
	}
	if len(uids) != 1 {
		t.Fatalf("expected 1 uid, got %d", len(uids))
	}

	data, err := client.FetchSenderData(ctx, uids)
	if err != nil {
		t.Fatalf("fetch sender data: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 message, got %d", len(data))
	}
	if data[0].ReturnPathDomain != "srs.messagingengine.com" {
		t.Fatalf("expected return-path domain, got %q", data[0].ReturnPathDomain)
	}
}

func TestFetchSenderDataReturnsErrorOnFetchFailure(t *testing.T) {
	addr, cleanup := ftest.SetupAnalyzeIMAPServer(t, []ftest.AnalyzeMessage{
		{
			From:    "News <news@example.com>",
			To:      "User <user@example.com>",
			Subject: "Hello",
			ListID:  "",
			Body:    "Body",
			Time:    time.Now().Add(-2 * time.Hour),
		},
	})
	t.Cleanup(cleanup)

	client := &Client{
		Addr:      addr,
		Username:  "user@example.com",
		Password:  "password",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if _, err := client.SelectMailbox(ctx, "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		t.Fatalf("search uids: %v", err)
	}
	if len(uids) != 1 {
		t.Fatalf("expected 1 uid, got %d", len(uids))
	}

	cleanup()
	_, err = client.FetchSenderData(ctx, uids)
	if err == nil {
		t.Fatal("expected fetch error after server shutdown")
	}
}

func TestReadHeaderLiteralFailure(t *testing.T) {
	_, err := readHeader(errorLiteral{})
	if err == nil {
		t.Fatal("expected readHeader to return error")
	}
}

func TestAnalyzeAgeWindowEndToEnd(t *testing.T) {
	addr, cleanup := ftest.SetupAnalyzeIMAPServer(t, []ftest.AnalyzeMessage{
		{
			From:    "News <news@example.com>",
			To:      "User <user@example.com>",
			Subject: "Recent",
			ListID:  "list.recent.example.com",
			Body:    "unsubscribe",
			Time:    time.Now().Add(-48 * time.Hour),
		},
		{
			From:    "Old <old@example.com>",
			To:      "User <user@example.com>",
			Subject: "Old",
			ListID:  "list.old.example.com",
			Body:    "unsubscribe",
			Time:    time.Now().Add(-10 * 24 * time.Hour),
		},
	})
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
  - name: "Rule"
    server:
      age_window:
        min: "24h"
        max: "7d"
      folders:
        - "INBOX"
      body_substring:
        - "unsubscribe"
    actions: []
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("validate config: %v", err)
	}

	client := &Client{
		Addr:      addr,
		Username:  "user@example.com",
		Password:  "password",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	rule := cfg.Rules[0]
	matched, err := client.SearchByServerMatchers(ctx, *rule.Server)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	uids := matched["INBOX"]
	if len(uids) != 1 {
		t.Fatalf("expected 1 matched message, got %d", len(uids))
	}
}

func fetchMessageFlags(ctx context.Context, client *Client, uids []uint32) ([]imap.Flag, error) {
	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}

	fetchOptions := &imap.FetchOptions{
		Flags: true,
	}

	fetchCmd := client.client.Fetch(uidSet, fetchOptions)
	for {
		if err := ctx.Err(); err != nil {
			_ = fetchCmd.Close()
			return nil, err
		}
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		for {
			item := msg.Next()
			if item == nil {
				break
			}
			if data, ok := item.(giimapclient.FetchItemDataFlags); ok {
				return data.Flags, nil
			}
		}
	}
	if err := fetchCmd.Close(); err != nil {
		return nil, err
	}
	return nil, nil
}

func containsFlag(flags []imap.Flag, target imap.Flag) bool {
	for _, flag := range flags {
		if flag == target {
			return true
		}
	}
	return false
}

type errorLiteral struct{}

func (errorLiteral) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (errorLiteral) Size() int64 {
	return 0
}

func splitHostPort(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}
	return host, port, nil
}
