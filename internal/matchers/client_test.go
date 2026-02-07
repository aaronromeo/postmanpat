package matchers

import (
	"testing"

	"github.com/aaronromeo/postmanpat/internal/config"
)

func TestMatchesClientListIDRegex(t *testing.T) {
	matchers := &config.ClientMatchers{
		ListIDRegex: []string{`<f7443300a7bb349db1e85fa6e\.1520313\.list-id\.mcsv\.net>`},
	}
	data := ClientMessage{
		ListID: "f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	}

	ok, err := MatchesClient(matchers, data)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected list_id_regex to match ListID")
	}
}

func TestMatchesClientListIDRegexNoMatch(t *testing.T) {
	matchers := &config.ClientMatchers{
		ListIDRegex: []string{`<not-a-match>`},
	}
	data := ClientMessage{
		ListID: "f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	}

	ok, err := MatchesClient(matchers, data)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected list_id_regex to not match ListID")
	}
}

func TestMatchesClientSenderAndReplyToRegex(t *testing.T) {
	matchers := &config.ClientMatchers{
		SenderRegex:  []string{`ghost\.io`},
		ReplyToRegex: []string{`404media\.co`},
	}
	data := ClientMessage{
		SenderDomains:  []string{"news.ghost.io"},
		ReplyToDomains: []string{"404media.co"},
	}

	ok, err := MatchesClient(matchers, data)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected sender_regex and replyto_regex to both match")
	}
}

func TestMatchesClientSenderAndReplyToRegexRequiresBoth(t *testing.T) {
	matchers := &config.ClientMatchers{
		SenderRegex:  []string{`ghost\.io`},
		ReplyToRegex: []string{`404media\.co`},
	}
	data := ClientMessage{
		SenderDomains:  []string{"news.ghost.io"},
		ReplyToDomains: []string{"example.com"},
	}

	ok, err := MatchesClient(matchers, data)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected sender_regex and replyto_regex to require both matches")
	}
}

func TestMatchesClientSubjectRegex(t *testing.T) {
	matchers := &config.ClientMatchers{
		SubjectRegex: []string{`(?i)welcome`},
	}
	data := ClientMessage{
		SubjectRaw: "Welcome to the list",
	}

	ok, err := MatchesClient(matchers, data)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected subject_regex to match subject")
	}
}

func TestMatchesClientRecipientsRegex(t *testing.T) {
	matchers := &config.ClientMatchers{
		RecipientsRegex: []string{`user@example\.com`},
	}
	data := ClientMessage{
		Recipients: []string{"user@example.com"},
	}

	ok, err := MatchesClient(matchers, data)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected recipients_regex to match recipient")
	}
}
