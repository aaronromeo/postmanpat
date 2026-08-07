package cli

import (
	"testing"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
)

func ignoreTestMailData(from string, domains []string, listID string, tags []string, subjectRaw string) imap.MailData {
	return imap.MailData{
		From:          []string{from},
		SenderDomains: domains,
		ListID:        listID,
		RecipientTags: tags,
		SubjectRaw:    subjectRaw,
		MessageDate:   time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
	}
}

func TestMatchesIgnoreMatchersEmptyMatchers(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "", nil, "Hello")
	if matchesIgnoreMatchers(item, appconfig.IgnoreMatchers{}) {
		t.Fatal("empty matchers should not match")
	}
}

func TestMatchesIgnoreMatchersSenderDomainExact(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderDomains: []string{"github.com"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("exact domain should match")
	}
}

func TestMatchesIgnoreMatchersSenderDomainNotLookalike(t *testing.T) {
	item := ignoreTestMailData("user@notgithub.com", []string{"notgithub.com"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderDomains: []string{"github.com"}}
	if matchesIgnoreMatchers(item, m) {
		t.Fatal("notgithub.com should not match github.com")
	}
}

func TestMatchesIgnoreMatchersSenderDomainNotSubdomain(t *testing.T) {
	item := ignoreTestMailData("user@emails.github.com", []string{"emails.github.com"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderDomains: []string{"github.com"}}
	if matchesIgnoreMatchers(item, m) {
		t.Fatal("subdomain should not match parent domain")
	}
}

func TestMatchesIgnoreMatchersSenderDomainAnyElement(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com", "other.com"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderDomains: []string{"other.com"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("should match any sender domain")
	}
}

func TestMatchesIgnoreMatchersSenderDomainCaseInsensitive(t *testing.T) {
	item := ignoreTestMailData("user@GitHub.COM", []string{"GitHub.COM"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderDomains: []string{"github.com"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("domain match should be case-insensitive")
	}
}

func TestMatchesIgnoreMatchersSenderAddressSubstring(t *testing.T) {
	item := ignoreTestMailData("newsletter@github.com", []string{"github.com"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderAddresses: []string{"newsletter"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("sender address substring should match")
	}
}

func TestMatchesIgnoreMatchersSenderAddressCaseInsensitive(t *testing.T) {
	item := ignoreTestMailData("Newsletter@github.com", []string{"github.com"}, "", nil, "Hello")
	m := appconfig.IgnoreMatchers{SenderAddresses: []string{"newsletter"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("sender address match should be case-insensitive")
	}
}

func TestMatchesIgnoreMatchersListIDSubstring(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "<announce.github.com>", nil, "Hello")
	m := appconfig.IgnoreMatchers{ListIDs: []string{"announce"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("list-id substring should match")
	}
}

func TestMatchesIgnoreMatchersListIDCaseInsensitive(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "<Announce.GitHub.com>", nil, "Hello")
	m := appconfig.IgnoreMatchers{ListIDs: []string{"announce"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("list-id match should be case-insensitive")
	}
}

func TestMatchesIgnoreMatchersRecipientTagSubstring(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "", []string{"plus-tag-123"}, "Hello")
	m := appconfig.IgnoreMatchers{RecipientTags: []string{"plus-tag"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("recipient tag substring should match")
	}
}

func TestMatchesIgnoreMatchersRecipientTagCaseInsensitive(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "", []string{"Plus-Tag-123"}, "Hello")
	m := appconfig.IgnoreMatchers{RecipientTags: []string{"plus-tag"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("recipient tag match should be case-insensitive")
	}
}

func TestMatchesIgnoreMatchersSubjectSubstringRaw(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "", nil, "Your Weekly Digest")
	m := appconfig.IgnoreMatchers{SubjectSubstrings: []string{"weekly digest"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("subject substring should match raw subject")
	}
}

func TestMatchesIgnoreMatchersSubjectRawNotNormalized(t *testing.T) {
	item := imap.MailData{
		From:              []string{"user@github.com"},
		SenderDomains:     []string{"github.com"},
		SubjectRaw:        "Order #12345 has shipped",
		SubjectNormalized: "order #{{n}} has shipped",
		MessageDate:       time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
	}
	m := appconfig.IgnoreMatchers{SubjectSubstrings: []string{"#12345"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("should match against raw subject, not normalized")
	}
}

func TestMatchesIgnoreMatchersAnyFieldMatches(t *testing.T) {
	item := ignoreTestMailData("user@github.com", []string{"github.com"}, "<list.example.com>", []string{"newsletter"}, "Hello World")
	m := appconfig.IgnoreMatchers{ListIDs: []string{"list.example"}}
	if !matchesIgnoreMatchers(item, m) {
		t.Fatal("should match when any single field matches")
	}
}

func TestFilterFullyDecidedNilIgnore(t *testing.T) {
	data := []imap.MailData{ignoreTestMailData("a@x.com", []string{"x.com"}, "", nil, "Hi")}
	if got := filterFullyDecided(data, nil); len(got) != 1 {
		t.Fatalf("nil ignore should return data unchanged, got %d items", len(got))
	}
}

func TestFilterFullyDecidedWatchOnlyRetained(t *testing.T) {
	data := []imap.MailData{ignoreTestMailData("a@x.com", []string{"x.com"}, "", nil, "Hi")}
	ignore := &appconfig.IgnoreConfig{
		Watch:   appconfig.IgnoreMatchers{SenderDomains: []string{"x.com"}},
		Cleanup: appconfig.IgnoreMatchers{},
	}
	if got := filterFullyDecided(data, ignore); len(got) != 1 {
		t.Fatal("watch-only match should be retained")
	}
}

func TestFilterFullyDecidedCleanupOnlyRetained(t *testing.T) {
	data := []imap.MailData{ignoreTestMailData("a@x.com", []string{"x.com"}, "", nil, "Hi")}
	ignore := &appconfig.IgnoreConfig{
		Watch:   appconfig.IgnoreMatchers{},
		Cleanup: appconfig.IgnoreMatchers{SenderDomains: []string{"x.com"}},
	}
	if got := filterFullyDecided(data, ignore); len(got) != 1 {
		t.Fatal("cleanup-only match should be retained")
	}
}

func TestFilterFullyDecidedBothListsFiltered(t *testing.T) {
	data := []imap.MailData{
		ignoreTestMailData("a@x.com", []string{"x.com"}, "", nil, "Hi"),
		ignoreTestMailData("b@y.com", []string{"y.com"}, "", nil, "Bye"),
	}
	ignore := &appconfig.IgnoreConfig{
		Watch:   appconfig.IgnoreMatchers{SenderDomains: []string{"x.com"}},
		Cleanup: appconfig.IgnoreMatchers{SenderDomains: []string{"x.com"}},
	}
	got := filterFullyDecided(data, ignore)
	if len(got) != 1 {
		t.Fatalf("both-lists match should be filtered, got %d items", len(got))
	}
	if got[0].From[0] != "b@y.com" {
		t.Fatal("remaining message should be the unfiltered one")
	}
}

func TestFilterFullyDecidedCrossIdentityBothLists(t *testing.T) {
	data := []imap.MailData{ignoreTestMailData("a@x.com", []string{"x.com"}, "<list.x.com>", nil, "Hi")}
	ignore := &appconfig.IgnoreConfig{
		Watch:   appconfig.IgnoreMatchers{SenderDomains: []string{"x.com"}},
		Cleanup: appconfig.IgnoreMatchers{ListIDs: []string{"list.x"}},
	}
	if got := filterFullyDecided(data, ignore); len(got) != 0 {
		t.Fatal("cross-identity both-lists match should be filtered (spec: conservative)")
	}
}
