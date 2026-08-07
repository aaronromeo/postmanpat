# Analyze Ignore List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a config-driven Ignore List (Watch Ignore List + Cleanup Ignore List) that filters Fully Decided messages out of `analyze` reports and suppresses per-rule-type prompts in `bin/postmanpat-generate-rules.py`.

**Architecture:** Split filtering per ADR 0001: `analyze` (Go) filters messages matching BOTH ignore lists before report aggregation (report stays silent; `--no-ignore` bypasses); the rule generator (Python) reads the same analyze YAML via a new `--config` flag and suppresses only the corresponding prompt for half-decided clusters, without touching the Generation Checkpoint.

**Tech Stack:** Go 1.25.5 (cobra, yaml.v3), Python 3.12 (stdlib + PyYAML), unittest (Python), in-memory IMAP server (ftest) for Go command-level tests.

**Spec:** `docs/superpowers/specs/2026-08-05-analyze-ignore-list-design.md` · **Glossary:** `CONTEXT.md` · **ADR:** `docs/adr/0001-ignore-list-split-filtering.md`

## Global Constraints

- Go 1.25.5 (pinned in `.tool-versions` and `go.mod`); Python 3 with stdlib + PyYAML only.
- No new third-party dependencies in Go or Python.
- `appconfig` tests use stdlib assertions (`t.Fatalf` + `strings.Contains`), NOT testify. `cli` tests follow the existing plain style of `cli/analyze_test.go`.
- No mocks in Go tests; command-level tests use the in-memory IMAP server (`ftest`).
- Conventional commit messages per `git log` (`feat(scope): ...`, `docs: ...`).
- All Go code gofmt-clean; keep the `if __name__ == "__main__"` guard in bin/ scripts (tests load them via importlib).
- IMAP logic stays vendor-neutral; no provider-specific assumptions.
- Terminology follows `CONTEXT.md` (Ignore List, Watch Ignore List, Cleanup Ignore List, Fully Decided, Generation Checkpoint).

**Task ordering:** Task 1 (appconfig) blocks Tasks 2–3 (Go cli). Tasks 4–6 (Python) are independent of the Go stream and can run in parallel with it. Task 7 (docs) comes last.

---

### Task 1: appconfig — ignore section model, load + validate

**Files:**
- Modify: `appconfig/config.go`
- Test: `appconfig/config_test.go`

**Interfaces:**
- Consumes: existing `Load(path)`, `Validate(cfg)`, `writeTempFile(t, yaml)` test helper.
- Produces (relied on by Tasks 2–3):
  ```go
  type IgnoreMatchers struct {
  	SenderDomains     []string `yaml:"sender_domains"`
  	SenderAddresses   []string `yaml:"sender_addresses"`
  	ListIDs           []string `yaml:"list_ids"`
  	RecipientTags     []string `yaml:"recipient_tags"`
  	SubjectSubstrings []string `yaml:"subject_substrings"`
  }
  type IgnoreConfig struct {
  	Watch   IgnoreMatchers `yaml:"watch"`
  	Cleanup IgnoreMatchers `yaml:"cleanup"`
  }
  // Config gains: Ignore *IgnoreConfig `yaml:"ignore"`
  ```
  Validation: nil/absent `ignore` is valid; empty subsections and empty lists are valid; whitespace-only entries are invalid with an error naming the field (e.g. `ignore.watch.sender_domains[1] must not be empty or whitespace`).

Notes for the implementer: `Validate(cfg Config)` takes Config by value and early-exits on empty rules — ignore validation goes after the existing rule checks. YAML decoding is non-strict (unknown keys silently ignored); that is accepted.

- [ ] **Step 1: Write the failing tests**

Append to `appconfig/config_test.go`:

```go
func TestIgnoreSectionAbsentIsValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore != nil {
		t.Fatalf("expected Ignore to be nil when section absent, got: %+v", cfg.Ignore)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass with no ignore section, got: %v", err)
	}
}

func TestIgnoreSectionParsesBothSubsections(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
    sender_addresses:
      - "noreply@github.com"
    list_ids:
      - "github-notifications"
    recipient_tags:
      - "dev"
    subject_substrings:
      - "[GitHub]"
  cleanup:
    sender_domains:
      - "mailer.example.com"
    sender_addresses:
      - "bulk@mailer.example.com"
    list_ids:
      - "weekly-digest"
    recipient_tags:
      - "promo"
    subject_substrings:
      - "Newsletter"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}

	w := cfg.Ignore.Watch
	if len(w.SenderDomains) != 1 || w.SenderDomains[0] != "github.com" {
		t.Fatalf("watch.sender_domains: got %v, want [github.com]", w.SenderDomains)
	}
	if len(w.SenderAddresses) != 1 || w.SenderAddresses[0] != "noreply@github.com" {
		t.Fatalf("watch.sender_addresses: got %v", w.SenderAddresses)
	}
	if len(w.ListIDs) != 1 || w.ListIDs[0] != "github-notifications" {
		t.Fatalf("watch.list_ids: got %v", w.ListIDs)
	}
	if len(w.RecipientTags) != 1 || w.RecipientTags[0] != "dev" {
		t.Fatalf("watch.recipient_tags: got %v", w.RecipientTags)
	}
	if len(w.SubjectSubstrings) != 1 || w.SubjectSubstrings[0] != "[GitHub]" {
		t.Fatalf("watch.subject_substrings: got %v", w.SubjectSubstrings)
	}

	c := cfg.Ignore.Cleanup
	if len(c.SenderDomains) != 1 || c.SenderDomains[0] != "mailer.example.com" {
		t.Fatalf("cleanup.sender_domains: got %v", c.SenderDomains)
	}
	if len(c.SenderAddresses) != 1 || c.SenderAddresses[0] != "bulk@mailer.example.com" {
		t.Fatalf("cleanup.sender_addresses: got %v", c.SenderAddresses)
	}
	if len(c.ListIDs) != 1 || c.ListIDs[0] != "weekly-digest" {
		t.Fatalf("cleanup.list_ids: got %v", c.ListIDs)
	}
	if len(c.RecipientTags) != 1 || c.RecipientTags[0] != "promo" {
		t.Fatalf("cleanup.recipient_tags: got %v", c.RecipientTags)
	}
	if len(c.SubjectSubstrings) != 1 || c.SubjectSubstrings[0] != "Newsletter" {
		t.Fatalf("cleanup.subject_substrings: got %v", c.SubjectSubstrings)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}

func TestIgnoreEmptyListsAreValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch: {}
  cleanup: {}
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass with empty lists, got: %v", err)
	}
}

func TestIgnoreWhitespaceOnlyEntryInvalid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
      - "   "
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error for whitespace-only entry")
	} else if !strings.Contains(err.Error(), "sender_domains") {
		t.Fatalf("expected error to mention 'sender_domains', got: %v", err)
	}
}

func TestIgnoreOnlyWatchPopulatedIsValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  watch:
    sender_domains:
      - "github.com"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}
	if len(cfg.Ignore.Watch.SenderDomains) != 1 {
		t.Fatal("expected watch.sender_domains to have 1 entry")
	}
	if len(cfg.Ignore.Cleanup.SenderDomains) != 0 {
		t.Fatalf("expected cleanup.sender_domains to be empty, got %v", cfg.Ignore.Cleanup.SenderDomains)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}

func TestIgnoreOnlyCleanupPopulatedIsValid(t *testing.T) {
	path := writeTempFile(t, `
rules:
  - name: "Rule"
    server:
      folders:
        - "INBOX"
    actions: []
ignore:
  cleanup:
    list_ids:
      - "weekly-digest"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.Ignore == nil {
		t.Fatal("expected Ignore to be non-nil")
	}
	if len(cfg.Ignore.Cleanup.ListIDs) != 1 {
		t.Fatal("expected cleanup.list_ids to have 1 entry")
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected validation to pass, got: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./appconfig/ -run "TestIgnore" -v`
Expected: FAIL — package does not compile (`undefined: IgnoreConfig` etc. is NOT shown because tests reference only `cfg.Ignore`; the compile error is `cfg.Ignore undefined (type Config has no field or method Ignore)`). All 6 tests fail.

- [ ] **Step 3: Write the minimal implementation**

In `appconfig/config.go`:

1. Add the `Ignore` field to the `Config` struct:

```go
type Config struct {
	Rules      []Rule        `yaml:"rules"`
	Checkpoint Checkpoint    `yaml:"checkpoint"`
	Ignore     *IgnoreConfig `yaml:"ignore"`
}
```

(Keep the existing field order/comments of `Config`; just add the `Ignore` line. If `Config` has doc comments on fields, match that style.)

2. Add the new structs after the `Checkpoint` struct definition:

```go
// IgnoreMatchers defines the identities ignored for one rule type.
// SenderDomains match exactly (case-insensitive); all other fields match as
// case-insensitive substrings.
type IgnoreMatchers struct {
	SenderDomains     []string `yaml:"sender_domains"`
	SenderAddresses   []string `yaml:"sender_addresses"`
	ListIDs           []string `yaml:"list_ids"`
	RecipientTags     []string `yaml:"recipient_tags"`
	SubjectSubstrings []string `yaml:"subject_substrings"`
}

// IgnoreConfig holds the Watch Ignore List and Cleanup Ignore List.
type IgnoreConfig struct {
	Watch   IgnoreMatchers `yaml:"watch"`
	Cleanup IgnoreMatchers `yaml:"cleanup"`
}
```

3. At the end of `Validate`, before its final `return nil`, add:

```go
	if err := validateIgnoreConfig(cfg.Ignore); err != nil {
		return err
	}
```

4. Add the validation helpers after `Validate`:

```go
func validateIgnoreConfig(ic *IgnoreConfig) error {
	if ic == nil {
		return nil
	}
	if err := validateIgnoreMatchers("ignore.watch", ic.Watch); err != nil {
		return err
	}
	if err := validateIgnoreMatchers("ignore.cleanup", ic.Cleanup); err != nil {
		return err
	}
	return nil
}

func validateIgnoreMatchers(prefix string, m IgnoreMatchers) error {
	fields := []struct {
		name   string
		values []string
	}{
		{"sender_domains", m.SenderDomains},
		{"sender_addresses", m.SenderAddresses},
		{"list_ids", m.ListIDs},
		{"recipient_tags", m.RecipientTags},
		{"subject_substrings", m.SubjectSubstrings},
	}
	for _, field := range fields {
		for i, v := range field.values {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%s.%s[%d] must not be empty or whitespace", prefix, field.name, i)
			}
		}
	}
	return nil
}
```

`strings` and `fmt` are already imported by `appconfig/config.go`; verify and add if needed.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./appconfig/ -run "TestIgnore" -v`
Expected: PASS (6 tests). Then run `go test ./appconfig/` — all pre-existing tests must still pass.

- [ ] **Step 5: Commit**

```bash
git add appconfig/config.go appconfig/config_test.go
git commit -m "feat(appconfig): add ignore section with validation"
```

---

### Task 2: cli — ignore matching + Fully Decided filtering (pure functions)

**Files:**
- Create: `cli/analyze_ignore.go`
- Test: `cli/analyze_ignore_test.go` (create)

**Interfaces:**
- Consumes: `appconfig.IgnoreMatchers`, `appconfig.IgnoreConfig`, `Config.Ignore *IgnoreConfig` (Task 1); `imap.MailData` — relevant fields: `From []string`, `SenderDomains []string`, `ListID string`, `RecipientTags []string`, `SubjectRaw string`.
- Produces (consumed by Task 3):
  ```go
  func matchesIgnoreMatchers(item imap.MailData, m appconfig.IgnoreMatchers) bool
  func filterFullyDecided(data []imap.MailData, ignore *appconfig.IgnoreConfig) []imap.MailData
  ```

Semantics: a message matches an `IgnoreMatchers` when ANY non-empty entry list matches (OR across fields). `SenderDomains`: exact, case-insensitive, any element of the message's sender domains. `SenderAddresses`/`ListIDs`/`RecipientTags`/`SubjectSubstrings`: case-insensitive substrings (addresses: any element of `From`; tags: any element of `RecipientTags`; subject: against `SubjectRaw`, never `SubjectNormalized`). `filterFullyDecided` drops a message only when it matches BOTH Watch and Cleanup; nil `ignore` returns data unchanged.

- [ ] **Step 1: Write the failing tests**

Create `cli/analyze_ignore_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cli/ -run "TestMatchesIgnoreMatchers|TestFilterFullyDecided" -v`
Expected: FAIL — package does not compile (`undefined: matchesIgnoreMatchers`, `undefined: filterFullyDecided`).

- [ ] **Step 3: Write the minimal implementation**

Create `cli/analyze_ignore.go`:

```go
package cli

import (
	"strings"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
)

// matchesIgnoreMatchers reports whether item matches any entry of m.
// Sender domains match exactly (case-insensitive); all other entry types
// match as case-insensitive substrings.
func matchesIgnoreMatchers(item imap.MailData, m appconfig.IgnoreMatchers) bool {
	return matchesIgnoreDomains(item.SenderDomains, m.SenderDomains) ||
		matchesIgnoreAddresses(item.From, m.SenderAddresses) ||
		matchesIgnoreString(item.ListID, m.ListIDs) ||
		matchesIgnoreTags(item.RecipientTags, m.RecipientTags) ||
		matchesIgnoreString(item.SubjectRaw, m.SubjectSubstrings)
}

func matchesIgnoreDomains(domains, entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		for _, domain := range domains {
			if strings.ToLower(strings.TrimSpace(domain)) == entry {
				return true
			}
		}
	}
	return false
}

func matchesIgnoreAddresses(addresses, entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		for _, addr := range addresses {
			if strings.Contains(strings.ToLower(addr), entry) {
				return true
			}
		}
	}
	return false
}

func matchesIgnoreTags(tags, entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag), entry) {
				return true
			}
		}
	}
	return false
}

func matchesIgnoreString(value string, entries []string) bool {
	if value == "" {
		return false
	}
	value = strings.ToLower(value)
	for _, entry := range entries {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(entry))) {
			return true
		}
	}
	return false
}

// filterFullyDecided removes messages that match BOTH the Watch Ignore List
// and the Cleanup Ignore List. A nil ignore config returns data unchanged.
func filterFullyDecided(data []imap.MailData, ignore *appconfig.IgnoreConfig) []imap.MailData {
	if ignore == nil {
		return data
	}
	filtered := make([]imap.MailData, 0, len(data))
	for _, item := range data {
		if matchesIgnoreMatchers(item, ignore.Watch) && matchesIgnoreMatchers(item, ignore.Cleanup) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cli/ -run "TestMatchesIgnoreMatchers|TestFilterFullyDecided" -v`
Expected: PASS (all tests). Then `gofmt -l cli/` prints nothing and `go vet ./cli/` is clean.

- [ ] **Step 5: Commit**

```bash
git add cli/analyze_ignore.go cli/analyze_ignore_test.go
git commit -m "feat(cli): add ignore matching and fully-decided filtering"
```

---

### Task 3: cli — wire filtering into analyze + `--no-ignore` flag (end-to-end)

**Files:**
- Modify: `cli/analyze.go` (flag registration in `init()`; flag read near the other option reads; filter call after `data := dataByMailbox[mailbox]`)
- Test: `cli/analyze_ignore_e2e_test.go` (create)

**Interfaces:**
- Consumes: `filterFullyDecided` (Task 2); `ftest.SetupAnalyzeIMAPServer(t *testing.T, messages []ftest.AnalyzeMessage) (string, func())` with `ftest.AnalyzeMessage{From, To, Subject, ListID, Body, Time}`; `ftest.DefaultUser`/`ftest.DefaultPass`; `splitHostPort(addr string) (string, string, error)` (already defined in `cli/watch_test.go`, same package).
- Produces: `analyze` honors `--no-ignore` (bool, default false, help: "Disable ignore-list filtering"); the written JSON report contains only un-ignored messages and no ignored-count fields.

Prior art for the test: `cli/watch_test.go` (env override via `t.Setenv`, `rootCmd.SetArgs`/`SetOut`, `rootCmd.Execute()`) and `cli/cleanup_trace_test.go` (imports `ftest`, uses `ftest.DefaultUser`).

- [ ] **Step 1: Write the failing end-to-end tests**

Create `cli/analyze_ignore_e2e_test.go`:

```go
package cli

import (
	"bytes"
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

	var output bytes.Buffer
	args := append([]string{"analyze", "--config", cfgPath}, extraArgs...)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cli/ -run "TestAnalyzeFiltersFullyDecidedMessages|TestAnalyzeNoIgnoreBypassesFiltering" -v`
Expected: FAIL — `TestAnalyzeNoIgnoreBypassesFiltering` errors with `unknown flag: --no-ignore`; `TestAnalyzeFiltersFullyDecidedMessages` gets `total_messages_scanned == 3` (filtering not wired).

- [ ] **Step 3: Wire the implementation**

In `cli/analyze.go`:

1. Register the flag in `init()`, after the `--min-count` line:

```go
	analyzeCmd.Flags().Bool("no-ignore", false, "Disable ignore-list filtering")
```

2. Read the flag alongside the other option reads (after the `minCount` block, before `options := analyzeOptions{...}`):

```go
	noIgnore, err := cmd.Flags().GetBool("no-ignore")
	if err != nil {
		return err
	}
```

3. In the rule loop, replace:

```go
		data := dataByMailbox[mailbox]
```

with:

```go
		data := dataByMailbox[mailbox]
		if !noIgnore {
			data = filterFullyDecided(data, cfg.Ignore)
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cli/ -run "TestAnalyzeFiltersFullyDecidedMessages|TestAnalyzeNoIgnoreBypassesFiltering" -v`
Expected: PASS (2 tests). Then run the full suite: `go test ./...` — everything green.

- [ ] **Step 5: Commit**

```bash
git add cli/analyze.go cli/analyze_ignore_e2e_test.go
git commit -m "feat(cli): filter fully-decided mail in analyze; add --no-ignore"
```

---

### Task 4: bin — `load_ignore_lists` for the rule generator

**Files:**
- Modify: `bin/postmanpat-generate-rules.py` (add `import yaml`, add function)
- Test: `bin/test_generate_rules.py` (create)

**Interfaces:**
- Consumes: path to the analyze YAML config (same file the Go side reads).
- Produces (consumed by Tasks 5–6):
  ```python
  def load_ignore_lists(config_path: str) -> Dict[str, Dict[str, List[str]]]:
      """Return {"watch": {field: [...]}, "cleanup": {field: [...]}} with all
      five fields (sender_domains, sender_addresses, list_ids, recipient_tags,
      subject_substrings) always present as lists. Missing file/section => empty."""
  ```

- [ ] **Step 1: Write the failing tests**

Create `bin/test_generate_rules.py`:

```python
#!/usr/bin/env python3
"""Tests for ignore-list loading and cluster suppression in generate-rules."""
import importlib.util
import os
import tempfile
import unittest
from pathlib import Path

BIN = Path(__file__).resolve().parent


def load_module(name: str):
    spec = importlib.util.spec_from_file_location(name, BIN / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


mod = load_module("postmanpat-generate-rules")

IGNORE_FIELDS = (
    "sender_domains",
    "sender_addresses",
    "list_ids",
    "recipient_tags",
    "subject_substrings",
)


class TestLoadIgnoreLists(unittest.TestCase):
    def _write_config(self, content: str) -> str:
        fd, path = tempfile.mkstemp(suffix=".yaml")
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(content)
        self.addCleanup(os.remove, path)
        return path

    def test_missing_config_returns_empty(self):
        result = mod.load_ignore_lists("/nonexistent/path.yaml")
        for side in ("watch", "cleanup"):
            for field in IGNORE_FIELDS:
                self.assertEqual(result[side][field], [])

    def test_no_ignore_section_returns_empty(self):
        path = self._write_config("rules: []\n")
        result = mod.load_ignore_lists(path)
        for side in ("watch", "cleanup"):
            for field in IGNORE_FIELDS:
                self.assertEqual(result[side][field], [])

    def test_watch_only_populates_watch(self):
        path = self._write_config(
            "ignore:\n"
            "  watch:\n"
            "    sender_domains: [github.com]\n"
            "    sender_addresses: [noreply@github.com]\n"
            "    list_ids: [github.lists]\n"
            "    recipient_tags: [tag1]\n"
            "    subject_substrings: [build failed]\n"
        )
        result = mod.load_ignore_lists(path)
        self.assertEqual(result["watch"]["sender_domains"], ["github.com"])
        self.assertEqual(result["watch"]["sender_addresses"], ["noreply@github.com"])
        self.assertEqual(result["watch"]["list_ids"], ["github.lists"])
        self.assertEqual(result["watch"]["recipient_tags"], ["tag1"])
        self.assertEqual(result["watch"]["subject_substrings"], ["build failed"])
        for field in IGNORE_FIELDS:
            self.assertEqual(result["cleanup"][field], [])

    def test_fully_populated(self):
        path = self._write_config(
            "ignore:\n"
            "  watch:\n"
            "    sender_domains: [github.com]\n"
            "  cleanup:\n"
            "    sender_domains: [spam.example.com]\n"
            "    list_ids: [newsletter]\n"
            "    subject_substrings: [unsubscribe]\n"
        )
        result = mod.load_ignore_lists(path)
        self.assertEqual(result["watch"]["sender_domains"], ["github.com"])
        self.assertEqual(result["cleanup"]["sender_domains"], ["spam.example.com"])
        self.assertEqual(result["cleanup"]["list_ids"], ["newsletter"])
        self.assertEqual(result["cleanup"]["subject_substrings"], ["unsubscribe"])
        self.assertEqual(result["watch"]["sender_addresses"], [])
        self.assertEqual(result["watch"]["list_ids"], [])
        self.assertEqual(result["cleanup"]["sender_addresses"], [])

    def test_empty_lists_stay_empty(self):
        path = self._write_config("ignore:\n  watch: {}\n  cleanup: {}\n")
        result = mod.load_ignore_lists(path)
        for side in ("watch", "cleanup"):
            for field in IGNORE_FIELDS:
                self.assertEqual(result[side][field], [])


if __name__ == "__main__":
    unittest.main(verbosity=2)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 bin/test_generate_rules.py`
Expected: FAIL — `AttributeError: module ... has no attribute 'load_ignore_lists'`.

- [ ] **Step 3: Write the minimal implementation**

In `bin/postmanpat-generate-rules.py`:

1. Add the yaml import after the existing `from typing import ...` line (matching the style used in `bin/postmanpat-convert-watch-to-cleanup.py`):

```python
import yaml  # type: ignore
```

2. Add the function (place it near `load_checkpoint`):

```python
def load_ignore_lists(config_path: str) -> Dict[str, Dict[str, List[str]]]:
    """Return {"watch": {field: [...]}, "cleanup": {...}}; all five fields
    always present as lists (possibly empty). Missing file/section => empty."""
    fields = (
        "sender_domains",
        "sender_addresses",
        "list_ids",
        "recipient_tags",
        "subject_substrings",
    )
    result: Dict[str, Dict[str, List[str]]] = {
        side: {f: [] for f in fields} for side in ("watch", "cleanup")
    }
    if not os.path.exists(config_path):
        return result
    with open(config_path, "r", encoding="utf-8") as fh:
        cfg = yaml.safe_load(fh)
    if not isinstance(cfg, dict):
        return result
    ignore = cfg.get("ignore")
    if not isinstance(ignore, dict):
        return result
    for side in ("watch", "cleanup"):
        section = ignore.get(side)
        if not isinstance(section, dict):
            continue
        for field in fields:
            value = section.get(field)
            if isinstance(value, list):
                result[side][field] = [str(v) for v in value]
    return result
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python3 bin/test_generate_rules.py`
Expected: PASS (5 tests). Existing `python3 bin/test_yaml_scalar.py` still passes.

- [ ] **Step 5: Commit**

```bash
git add bin/postmanpat-generate-rules.py bin/test_generate_rules.py
git commit -m "feat(bin): load ignore lists from analyze config"
```

---

### Task 5: bin — `cluster_rule_suppression` per-lens matching

**Files:**
- Modify: `bin/postmanpat-generate-rules.py` (add function)
- Test: `bin/test_generate_rules.py` (add test class)

**Interfaces:**
- Consumes: `load_ignore_lists` output shape (Task 4). Cluster dicts as written by analyze: `keys["ListID"]` (str, list_lens), `keys["SenderDomains"]` (list[str], sender_unsub_lens), `keys["FromList"]` (list[str], sender_unsub_lens), `keys["recipient_tag"]` (str, recipient_tag_lens), `examples["subject_raw"]` (list[str], all lenses).
- Produces (consumed by Task 6):
  ```python
  def cluster_rule_suppression(cluster: dict, lens_name: str, ignore_lists: dict) -> tuple[bool, bool]:
      """Return (suppress_watch, suppress_cleanup) for one cluster."""
  ```

Matching: `sender_domains` exact (case-insensitive, any element of `SenderDomains`); `sender_addresses` substring against any `FromList` element; `list_ids` substring against `ListID`; `recipient_tags` substring against `recipient_tag`; `subject_substrings` best-effort substring against `examples.subject_raw`. A side suppresses when ANY of its entries matches. (`lens_name` is accepted for call-site clarity; key presence per lens makes the matching lens-specific naturally.)

- [ ] **Step 1: Write the failing tests**

Add to `bin/test_generate_rules.py` (before the `if __name__` guard):

```python
class TestClusterRuleSuppression(unittest.TestCase):
    def _ignore(self, watch=None, cleanup=None):
        result = {side: {f: [] for f in IGNORE_FIELDS} for side in ("watch", "cleanup")}
        if watch:
            result["watch"].update(watch)
        if cleanup:
            result["cleanup"].update(cleanup)
        return result

    def _cluster(self, keys=None, examples=None):
        return {
            "cluster_id": "test:abc123",
            "count": 5,
            "keys": keys or {},
            "examples": examples or {},
        }

    def test_list_lens_watch_suppressed_by_list_id(self):
        cluster = self._cluster(keys={"ListID": "github.lists.announcements"})
        ignore = self._ignore(watch={"list_ids": ["github.lists"]})
        sw, sc = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertTrue(sw)
        self.assertFalse(sc)

    def test_list_lens_cleanup_suppressed_by_list_id(self):
        cluster = self._cluster(keys={"ListID": "github.lists.announcements"})
        ignore = self._ignore(cleanup={"list_ids": ["github.lists"]})
        sw, sc = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertFalse(sw)
        self.assertTrue(sc)

    def test_list_lens_case_insensitive(self):
        cluster = self._cluster(keys={"ListID": "GitHub.LISTS.Announcements"})
        ignore = self._ignore(watch={"list_ids": ["github.lists"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertTrue(sw)

    def test_list_lens_no_match(self):
        cluster = self._cluster(keys={"ListID": "unique.sender.news"})
        ignore = self._ignore(watch={"list_ids": ["github.lists"]})
        sw, sc = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertFalse(sw)
        self.assertFalse(sc)

    def test_sender_unsub_watch_suppressed_by_exact_domain(self):
        cluster = self._cluster(keys={"SenderDomains": ["github.com", "actions.github.com"]})
        ignore = self._ignore(watch={"sender_domains": ["github.com"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "sender_unsub_lens", ignore)
        self.assertTrue(sw)

    def test_sender_unsub_no_partial_domain_match(self):
        cluster = self._cluster(keys={"SenderDomains": ["notgithub.com"]})
        ignore = self._ignore(watch={"sender_domains": ["github.com"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "sender_unsub_lens", ignore)
        self.assertFalse(sw)

    def test_sender_unsub_case_insensitive_exact(self):
        cluster = self._cluster(keys={"SenderDomains": ["GitHub.COM"]})
        ignore = self._ignore(watch={"sender_domains": ["github.com"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "sender_unsub_lens", ignore)
        self.assertTrue(sw)

    def test_sender_unsub_suppressed_by_from_address_substring(self):
        cluster = self._cluster(keys={"FromList": ["noreply@github.com", "alerts@github.com"]})
        ignore = self._ignore(watch={"sender_addresses": ["noreply@github"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "sender_unsub_lens", ignore)
        self.assertTrue(sw)

    def test_recipient_tag_suppressed(self):
        cluster = self._cluster(keys={"recipient_tag": "newsletter,weekly-digest"})
        ignore = self._ignore(cleanup={"recipient_tags": ["newsletter"]})
        sw, sc = mod.cluster_rule_suppression(cluster, "recipient_tag_lens", ignore)
        self.assertFalse(sw)
        self.assertTrue(sc)

    def test_subject_suppressed_via_examples(self):
        cluster = self._cluster(
            keys={"ListID": "some.list"},
            examples={"subject_raw": ["Your weekly build failed report"]},
        )
        ignore = self._ignore(watch={"subject_substrings": ["build failed"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertTrue(sw)

    def test_subject_case_insensitive(self):
        cluster = self._cluster(
            keys={"ListID": "some.list"},
            examples={"subject_raw": ["BUILD FAILED: project-x"]},
        )
        ignore = self._ignore(watch={"subject_substrings": ["build failed"]})
        sw, _ = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertTrue(sw)

    def test_subject_no_match_when_examples_empty(self):
        cluster = self._cluster(keys={"ListID": "some.list"}, examples={})
        ignore = self._ignore(watch={"subject_substrings": ["build failed"]})
        sw, sc = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertFalse(sw)
        self.assertFalse(sc)

    def test_both_lists_suppressed(self):
        cluster = self._cluster(keys={"ListID": "newsletter.weekly"})
        ignore = self._ignore(
            watch={"list_ids": ["newsletter"]},
            cleanup={"list_ids": ["newsletter"]},
        )
        sw, sc = mod.cluster_rule_suppression(cluster, "list_lens", ignore)
        self.assertTrue(sw)
        self.assertTrue(sc)

    def test_empty_ignore_no_suppression(self):
        cluster = self._cluster(keys={"ListID": "anything"})
        sw, sc = mod.cluster_rule_suppression(cluster, "list_lens", self._ignore())
        self.assertFalse(sw)
        self.assertFalse(sc)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 bin/test_generate_rules.py`
Expected: FAIL — `AttributeError: module ... has no attribute 'cluster_rule_suppression'`.

- [ ] **Step 3: Write the minimal implementation**

Add to `bin/postmanpat-generate-rules.py` (after `load_ignore_lists`):

```python
def _matches_ignore_side(keys: Dict[str, Any], examples: Dict[str, Any], side: Dict[str, List[str]]) -> bool:
    """True when the cluster matches ANY entry of one ignore side."""
    domains = keys.get("SenderDomains")
    if domains and side.get("sender_domains"):
        cluster_domains = {str(d).lower() for d in domains}
        if any(str(e).lower() in cluster_domains for e in side["sender_domains"]):
            return True

    from_list = keys.get("FromList")
    if from_list and side.get("sender_addresses"):
        for entry in side["sender_addresses"]:
            needle = str(entry).lower()
            if any(needle in str(addr).lower() for addr in from_list):
                return True

    list_id = str(keys.get("ListID") or "")
    if list_id and side.get("list_ids"):
        for entry in side["list_ids"]:
            if str(entry).lower() in list_id.lower():
                return True

    tag = str(keys.get("recipient_tag") or "")
    if tag and side.get("recipient_tags"):
        for entry in side["recipient_tags"]:
            if str(entry).lower() in tag.lower():
                return True

    subjects = examples.get("subject_raw") or []
    if subjects and side.get("subject_substrings"):
        for entry in side["subject_substrings"]:
            needle = str(entry).lower()
            if any(needle in str(s).lower() for s in subjects):
                return True

    return False


def cluster_rule_suppression(cluster: dict, lens_name: str, ignore_lists: dict) -> tuple[bool, bool]:
    """Return (suppress_watch, suppress_cleanup) for one cluster. Matching is
    against Lens Keys (plus best-effort subject matching via examples); key
    presence per lens makes the match lens-specific."""
    keys = cluster.get("keys") or {}
    examples = cluster.get("examples") or {}
    suppress_watch = _matches_ignore_side(keys, examples, ignore_lists.get("watch", {}))
    suppress_cleanup = _matches_ignore_side(keys, examples, ignore_lists.get("cleanup", {}))
    return suppress_watch, suppress_cleanup
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python3 bin/test_generate_rules.py`
Expected: PASS (all tests: Task 4's 5 + this task's 14).

- [ ] **Step 5: Commit**

```bash
git add bin/postmanpat-generate-rules.py bin/test_generate_rules.py
git commit -m "feat(bin): add per-cluster rule suppression matching"
```

---

### Task 6: bin — wire `--config` + suppression into the generator main loop

**Files:**
- Modify: `bin/postmanpat-generate-rules.py` (argparse `--config`; rename `process_cluster` → `process_cluster_prompts` with `ask_watch`/`ask_cleanup`; main loop suppression + skip summary)
- Test: `bin/test_generate_rules.py` (add `TestProcessClusterPromptsSuppression`)

**Interfaces:**
- Consumes: `load_ignore_lists` (Task 4), `cluster_rule_suppression` (Task 5), existing helpers `cluster_summary`, `format_examples`, `prompt_yes_no`, `rule_name_prompt`, `build_watch_rule_list/sender/recipient_tag`, `build_cleanup_rule_list/sender`, `save_checkpoint`.
- Produces:
  ```python
  def process_cluster_prompts(
      lens: str,
      cluster: Dict[str, Any],
      watch_rules: List[Dict[str, Any]],
      cleanup_rules: List[Dict[str, Any]],
      default_folders: Optional[str],
      ask_watch: bool = True,
      ask_cleanup: bool = True,
  ) -> Tuple[bool, Optional[str]]:
  ```
  New optional CLI arg `--config`. Without `--config`, behavior is byte-for-byte identical to today. Suppressed clusters are never written to the Generation Checkpoint.

- [ ] **Step 1: Write the failing tests**

Add to `bin/test_generate_rules.py` (before the `if __name__` guard; add `from unittest import mock` to the test file's imports):

```python
class TestProcessClusterPromptsSuppression(unittest.TestCase):
    def _cluster(self):
        return {
            "cluster_id": "list_lens:abc",
            "count": 5,
            "latest_date": "2026-08-01T00:00:00Z",
            "keys": {"ListID": "some.list"},
            "signals": {"has_list_id": True, "has_list_unsubscribe": True, "precedence_categories": {}},
            "examples": {"subject_raw": ["Hello"], "recipients": [], "reply_to_domains": [],
                         "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []},
        }

    def _run_prompts(self, ask_watch=True, ask_cleanup=True):
        asked = []

        def fake_prompt_yes_no(message, default=False):
            asked.append(message)
            return "n"

        watch_rules: list = []
        cleanup_rules: list = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt_yes_no):
            proceed, _ = mod.process_cluster_prompts(
                "list_lens",
                self._cluster(),
                watch_rules,
                cleanup_rules,
                "INBOX",
                ask_watch=ask_watch,
                ask_cleanup=ask_cleanup,
            )
        return proceed, asked, watch_rules, cleanup_rules

    def test_default_asks_both(self):
        proceed, asked, _, _ = self._run_prompts()
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)

    def test_watch_suppressed_skips_watch_prompt(self):
        proceed, asked, watch_rules, cleanup_rules = self._run_prompts(ask_watch=False)
        self.assertTrue(proceed)
        self.assertNotIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)
        self.assertEqual(watch_rules, [])
        self.assertEqual(cleanup_rules, [])

    def test_cleanup_suppressed_skips_cleanup_prompt(self):
        proceed, asked, watch_rules, cleanup_rules = self._run_prompts(ask_cleanup=False)
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertNotIn("Generate cleanup rule?", asked)
        self.assertEqual(watch_rules, [])
        self.assertEqual(cleanup_rules, [])
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 bin/test_generate_rules.py`
Expected: FAIL — `AttributeError: module ... has no attribute 'process_cluster_prompts'`.

- [ ] **Step 3: Write the implementation**

In `bin/postmanpat-generate-rules.py`:

1. Replace the whole existing `process_cluster` function (currently lines 313–362) with:

```python
def process_cluster_prompts(
    lens: str,
    cluster: Dict[str, Any],
    watch_rules: List[Dict[str, Any]],
    cleanup_rules: List[Dict[str, Any]],
    default_folders: Optional[str],
    ask_watch: bool = True,
    ask_cleanup: bool = True,
) -> Tuple[bool, Optional[str]]:
    print("\n=== Cluster ===")
    print(cluster_summary(cluster))
    for line in format_examples(cluster):
        print(f"  {line}")

    rule_name: Optional[str] = None

    if ask_watch:
        watch_response = prompt_yes_no("Generate watch rule?", default=False)
        if watch_response == "q":
            return False, default_folders
        if watch_response == "y":
            rule_name = rule_name or rule_name_prompt(lens, cluster)
            if lens == "list_lens":
                rule = build_watch_rule_list(cluster, rule_name)
            elif lens == "sender_unsub_lens":
                rule = build_watch_rule_sender(cluster, rule_name)
            elif lens == "recipient_tag_lens":
                rule = build_watch_rule_recipient_tag(cluster, rule_name)
            else:
                print(f"Unsupported lens for watch rules: {lens}")
                rule = None
            if rule:
                watch_rules.append(rule)
    else:
        print("Watch rule suppressed by ignore list.")

    if ask_cleanup:
        cleanup_response = prompt_yes_no("Generate cleanup rule?", default=False)
        if cleanup_response == "q":
            return False, default_folders
        if cleanup_response == "y":
            rule_name = rule_name or rule_name_prompt(lens, cluster)
            if lens == "list_lens":
                rule, default_folders = build_cleanup_rule_list(cluster, rule_name, default_folders)
            elif lens == "sender_unsub_lens":
                rule, default_folders = build_cleanup_rule_sender(cluster, rule_name, default_folders)
            elif lens == "recipient_tag_lens":
                print("recipient_tag_lens does not support server-side cleanup rules.")
                rule = None
            else:
                print(f"Unsupported lens for cleanup rules: {lens}")
                rule = None
            if rule:
                cleanup_rules.append(rule)
    else:
        print("Cleanup rule suppressed by ignore list.")

    return True, default_folders
```

2. Add the `--config` argument in `main()`, after the `--checkpoint` argument block:

```python
    parser.add_argument(
        "--config",
        default=None,
        help="Path to analyze YAML config with ignore section (enables prompt suppression)",
    )
```

3. In `main()`, after the `checkpoint_path = ...` line, add:

```python
    ignore_lists = load_ignore_lists(args.config) if args.config else None
```

4. Replace the main cluster loop:

```python
    for lens, cluster in clusters:
        if not enabled_lenses.get(lens, True):
            continue
        cluster_id = str(cluster.get("cluster_id", ""))
        if cluster_id in processed_ids:
            continue

        proceed, default_folders = process_cluster(
            lens,
            cluster,
            watch_rules,
            cleanup_rules,
            default_folders,
        )
        if not proceed:
            break
        processed_ids.add(cluster_id)
        save_checkpoint(checkpoint_path, processed_ids)
```

with:

```python
    suppressed_count = 0

    for lens, cluster in clusters:
        if not enabled_lenses.get(lens, True):
            continue
        cluster_id = str(cluster.get("cluster_id", ""))
        if cluster_id in processed_ids:
            continue

        ask_watch, ask_cleanup = True, True
        if ignore_lists is not None:
            suppress_watch, suppress_cleanup = cluster_rule_suppression(cluster, lens, ignore_lists)
            if suppress_watch and suppress_cleanup:
                suppressed_count += 1
                continue  # not prompted -> not written to the Generation Checkpoint
            ask_watch, ask_cleanup = not suppress_watch, not suppress_cleanup

        proceed, default_folders = process_cluster_prompts(
            lens,
            cluster,
            watch_rules,
            cleanup_rules,
            default_folders,
            ask_watch=ask_watch,
            ask_cleanup=ask_cleanup,
        )
        if not proceed:
            break
        processed_ids.add(cluster_id)
        save_checkpoint(checkpoint_path, processed_ids)

    if suppressed_count:
        print(f"Skipped {suppressed_count} clusters suppressed by ignore list")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python3 bin/test_generate_rules.py`
Expected: PASS (all tests, including the 3 new prompt-flow tests). Also `python3 bin/test_yaml_scalar.py` still passes.

- [ ] **Step 5: Manual verification of the interactive flow**

```bash
cat > /tmp/pp-analyze.json << 'EOF'
{
  "generated_at": "2026-08-06T00:00:00Z",
  "source": {"mailbox": "INBOX", "account": "test@test.com", "time_window": {"after": "", "before": ""}},
  "stats": {"total_messages_scanned": 10},
  "indexes": {
    "list_lens": {
      "key_fields": ["ListID"],
      "clusters": [
        {"cluster_id": "list_lens:aaa", "count": 5, "latest_date": "2026-08-01T00:00:00Z", "keys": {"ListID": "ignored.newsletter"}, "signals": {"has_list_id": true, "has_list_unsubscribe": true, "precedence_categories": {}}, "examples": {"subject_raw": ["News"], "recipients": [], "reply_to_domains": [], "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []}},
        {"cluster_id": "list_lens:bbb", "count": 3, "latest_date": "2026-08-01T00:00:00Z", "keys": {"ListID": "active.sender"}, "signals": {"has_list_id": true, "has_list_unsubscribe": true, "precedence_categories": {}}, "examples": {"subject_raw": ["Hi"], "recipients": [], "reply_to_domains": [], "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []}}
      ]
    },
    "sender_unsub_lens": {"key_fields": ["SenderDomains", "HasListUnsubscribe"], "clusters": []},
    "template_lens": {"key_fields": ["SenderDomains", "SubjectNormalized"], "clusters": []},
    "recipient_tag_lens": {"key_fields": ["recipient_tag"], "clusters": []}
  }
}
EOF

cat > /tmp/pp-config-watch-only.yaml << 'EOF'
ignore:
  watch:
    list_ids: ["ignored.newsletter"]
EOF

cat > /tmp/pp-config-both.yaml << 'EOF'
ignore:
  watch:
    list_ids: ["ignored.newsletter"]
  cleanup:
    list_ids: ["ignored.newsletter"]
EOF
```

Run A (watch-only suppression) — answer "y" to processing `list_lens`, then `n` to prompts:
`printf 'y\nn\nn\nn\n' | python3 bin/postmanpat-generate-rules.py --analyze /tmp/pp-analyze.json --watch-out /tmp/pp-watch.yml --cleanup-out /tmp/pp-cleanup.yml --config /tmp/pp-config-watch-only.yaml`
Expected: for cluster `ignored.newsletter`, "Watch rule suppressed by ignore list." prints and only "Generate cleanup rule?" is asked; `active.sender` asks both; no "Skipped N clusters" line.

Run B (both lists) :
`printf 'y\nn\nn\n' | python3 bin/postmanpat-generate-rules.py --analyze /tmp/pp-analyze.json --watch-out /tmp/pp-watch2.yml --cleanup-out /tmp/pp-cleanup2.yml --config /tmp/pp-config-both.yaml`
Expected: "Skipped 1 clusters suppressed by ignore list" prints; only `active.sender` is prompted.

Run C (checkpoint separation): after Run B, `cat /tmp/pp-watch2.yml.checkpoint.json` must contain only `list_lens:bbb` — NOT `list_lens:aaa`.

Run D (no --config): repeat Run A without `--config`; behavior is identical to today (both prompts for both clusters).

- [ ] **Step 6: Commit**

```bash
git add bin/postmanpat-generate-rules.py bin/test_generate_rules.py
git commit -m "feat(bin): wire ignore-list suppression into rule generator"
```

---

### Task 7: docs — README and AGENTS.md

**Files:**
- Modify: `README.md` (Docker Analyze section; rule-generation step)
- Modify: `AGENTS.md` (CLI Behavior, Project Structure `bin/` line)

**Interfaces:**
- Consumes: the finished feature from Tasks 1–6.
- Produces: documentation matching behavior.

- [ ] **Step 1: Update README.md**

In the "Docker (Analyze)" section, after the flags bullet line ("Flags: `--top` ... `--min-count` ..."), add:

```markdown
    - `--no-ignore`: disable ignore-list filtering for this run (audit what you're ignoring).
    - Optional top-level `ignore` section filters Fully Decided mail out of the report. Identities on both the `watch` and `cleanup` lists are removed before aggregation; identities on one list stay in the report and the rule generator suppresses only that rule type's prompt. `sender_domains` match exactly; all other fields are case-insensitive substrings (`subject_substrings` match raw subjects):

      ```yaml
      ignore:
        watch:
          sender_domains: ["github.com"]
        cleanup:
          sender_domains: ["github.com"]
          list_ids: ["weekly-digest"]
      ```
```

In "Turn the report into rules", update the script example to include the optional config:

```markdown
    ```bash
    python3 bin/postmanpat-generate-rules.py \
      --analyze analyze-out/postmanpat-analyze-*.json \
      --watch-out watch-new.yml \
      --cleanup-out cleanup-new.yml \
      --config config/config_analyze.yaml  # optional: enables ignore-list prompt suppression
    ```
```

- [ ] **Step 2: Update AGENTS.md**

In "CLI Behavior", extend the `analyze` bullet's flag list from ``(--top` `--examples` `--min-count`)`` to ``(--top` `--examples` `--min-count` `--no-ignore`)`` and append: "An optional top-level `ignore:` section (`watch:`/`cleanup:` sub-lists) filters Fully Decided messages (on both lists) out of the report; see `CONTEXT.md` and `docs/adr/0001-ignore-list-split-filtering.md`."

In "Project Structure", extend the `bin/` line to note `postmanpat-generate-rules.py` accepts `--config` for ignore-list prompt suppression.

- [ ] **Step 3: Verify and commit**

Run: `go test ./... && python3 bin/test_generate_rules.py && python3 bin/test_yaml_scalar.py`
Expected: all green (docs-only change; full verification pass).

```bash
git add README.md AGENTS.md
git commit -m "docs: document ignore list for analyze and rule generator"
```
