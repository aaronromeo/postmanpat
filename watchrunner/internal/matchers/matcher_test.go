package matchers

import (
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
)

func TestMatchesClientListIDRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		ListIDRegex: []string{`<f7443300a7bb349db1e85fa6e\.1520313\.list-id\.mcsv\.net>`},
	}
	data := ClientMessage{
		ListID: "f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected list_id_regex to match ListID")
	}
}

func TestMatchesClientListIDRegexNoMatch(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		ListIDRegex: []string{`<not-a-match>`},
	}
	data := ClientMessage{
		ListID: "f7443300a7bb349db1e85fa6emc list <f7443300a7bb349db1e85fa6e.1520313.list-id.mcsv.net>",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected list_id_regex to not match ListID")
	}
}

func TestMatchesClientSenderAndReplyToRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		SenderRegex:  []string{`ghost\.io`},
		ReplyToRegex: []string{`404media\.co`},
	}
	data := ClientMessage{
		SenderDomains:  []string{"news.ghost.io"},
		ReplyToDomains: []string{"404media.co"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected sender_regex and replyto_regex to both match")
	}
}

func TestMatchesClientSenderAndReplyToRegexRequiresBoth(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		SenderRegex:  []string{`ghost\.io`},
		ReplyToRegex: []string{`404media\.co`},
	}
	data := ClientMessage{
		SenderDomains:  []string{"news.ghost.io"},
		ReplyToDomains: []string{"example.com"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected sender_regex and replyto_regex to require both matches")
	}
}

func TestMatchesClientSubjectRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		SubjectRegex: []string{`(?i)welcome`},
	}
	data := ClientMessage{
		SubjectRaw: "Welcome to the list",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected subject_regex to match subject")
	}
}

func TestMatchesClientRecipientsRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		RecipientsRegex: []string{`user@example\.com`},
	}
	data := ClientMessage{
		Recipients: []string{"user@example.com"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected recipients_regex to match recipient")
	}
}

func TestMatchesClientCcRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		CcRegex: []string{`cc@example\.com`},
	}
	data := ClientMessage{
		Cc: []string{"cc@example.com"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected cc_regex to match cc recipient")
	}
}

func TestMatchesClientCcRegexNoMatch(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		CcRegex: []string{`cc@example\.com`},
	}
	data := ClientMessage{
		Cc: []string{"other@example.com"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected cc_regex to not match cc recipient")
	}
}

func TestMatchesClientReturnPathRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		ReturnPathRegex: []string{`srs\.messagingengine\.com`},
	}
	data := ClientMessage{
		ReturnPathDomain: "srs.messagingengine.com",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected returnpath_regex to match return-path domain")
	}
}

func TestMatchesClientReturnPathRegexNoMatch(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		ReturnPathRegex: []string{`srs\.messagingengine\.com`},
	}
	data := ClientMessage{
		ReturnPathDomain: "example.com",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected returnpath_regex to not match return-path domain")
	}
}

func TestMatchesClientBodyRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		BodyRegex: []string{`unsubscribe`},
	}
	data := ClientMessage{
		Body: "Click here to unsubscribe.",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected body_regex to match body")
	}
}

func TestMatchesClientListUnsubscribeTrue(t *testing.T) {
	listUnsub := true
	matchers := &appconfig.ClientMatchers{
		ListUnsubscribe: &listUnsub,
	}
	data := ClientMessage{
		ListUnsubscribe: true,
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected list_unsubscribe true to match")
	}
}

func TestMatchesClientListUnsubscribeFalse(t *testing.T) {
	listUnsub := false
	matchers := &appconfig.ClientMatchers{
		ListUnsubscribe: &listUnsub,
	}
	data := ClientMessage{
		ListUnsubscribe: true,
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected list_unsubscribe false to not match")
	}
}

func TestMatchesClientBodyRegexNoMatch(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		BodyRegex: []string{`unsubscribe`},
	}
	data := ClientMessage{
		Body: "Welcome to the list.",
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected body_regex to not match body")
	}
}

func TestMatchesClientRecipientTagRegex(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		RecipientTagRegex: []string{`news`},
	}
	data := ClientMessage{
		RecipientTags: []string{"news", "alerts"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if !ok {
		t.Fatal("expected recipient_tag_regex to match recipient tags")
	}
}

func TestMatchesClientRecipientTagRegexNoMatch(t *testing.T) {
	matchers := &appconfig.ClientMatchers{
		RecipientTagRegex: []string{`events`},
	}
	data := ClientMessage{
		RecipientTags: []string{"news", "alerts"},
	}

	ok, err := data.Match(matchers)
	if err != nil {
		t.Fatalf("match client: %v", err)
	}
	if ok {
		t.Fatal("expected recipient_tag_regex to not match recipient tags")
	}
}
