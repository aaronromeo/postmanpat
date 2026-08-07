# Analyze Ignore List Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a config-driven Ignore List (Watch Ignore List + Cleanup Ignore List) that filters Fully Decided messages out of `analyze` reports, annotates surviving clusters with per-rule-type suppression, and lets the rule generator read the annotation (no config-side matching) while interactively authoring new ignore entries.

**Architecture:** Split filtering per ADR 0001: `analyze` (Go) filters Fully Decided messages before aggregation and annotates each surviving cluster with `"suppressed": ["watch"]` and/or `["cleanup"]` (ADR 0002). The rule generator (Python) reads the annotation from the report JSON — zero ignore-matching logic — and can author new ignore entries into an `--ignore-out` YAML fragment via an `"i"` answer on prompts. `--no-ignore` disables both filtering and annotation.

**Tech Stack:** Go 1.25.5 (cobra, yaml.v3), Python 3.12 (stdlib + PyYAML), unittest (Python), in-memory IMAP server (ftest) for Go command-level tests.

**Spec:** `docs/superpowers/specs/2026-08-05-analyze-ignore-list-design.md` · **Glossary:** `CONTEXT.md` · **ADR:** `docs/adr/0001-ignore-list-split-filtering.md`, `docs/adr/0002-suppression-via-report-annotation.md`

## Global Constraints

- Go 1.25.5 (pinned in `.tool-versions` and `go.mod`); Python 3 with stdlib + PyYAML only.
- No new third-party dependencies in Go or Python.
- `appconfig` tests use stdlib assertions (`t.Fatalf` + `strings.Contains`), NOT testify. `cli` tests follow the existing plain style of `cli/analyze_test.go`.
- No mocks in Go tests; command-level tests use the in-memory IMAP server (`ftest`).
- Conventional commit messages per `git log` (`feat(scope): ...`, `docs: ...`).
- All Go code gofmt-clean; keep the `if __name__ == "__main__"` guard in bin/ scripts (tests load them via importlib).
- IMAP logic stays vendor-neutral; no provider-specific assumptions.
- Terminology follows `CONTEXT.md` (Ignore List, Watch Ignore List, Cleanup Ignore List, Fully Decided, Generation Checkpoint, Suppressed).
- JSON field `suppressed` uses `omitempty` — omitted when empty.

**Task ordering:** Task 1 (appconfig) blocks Tasks 2–4 (Go cli). Task 5 conceptually depends on Task 4 (annotation must exist) but is unit-testable with hand-built cluster dicts. Task 6 is independent of Tasks 4–5 (pure functions + prompt flow can be tested in isolation). Task 7 (docs) comes last.

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
Expected: FAIL — package does not compile (`cfg.Ignore undefined (type Config has no field or method Ignore)`). All 6 tests fail.

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

### Task 4: cli — per-cluster suppression annotation (TDD + e2e)

**Files:**
- Modify: `cli/analyze.go` (struct fields, accumulateCluster, builders, finalizeClusters, buildAnalyzeReport, RunE)
- Test: `cli/analyze_ignore_test.go` (add unit tests at buildAnalyzeReport seam)
- Test: `cli/analyze_ignore_e2e_test.go` (add e2e assertions for suppressed annotation)

**Interfaces:**
- Consumes: `matchesIgnoreMatchers` (Task 2), `appconfig.IgnoreConfig` (Task 1).
- Produces: `analyzeCluster` gains `Suppressed []string` (omitempty, deterministic order: watch first); `clusterAccumulator` gains `suppressWatch`, `suppressCleanup` bools; `accumulateCluster` gains `ignore *appconfig.IgnoreConfig` param; all four lens builders gain `ignore` param; `buildAnalyzeReport` params gain `Ignore`; `--no-ignore` disables both filtering and annotation.

Semantics: per-message during accumulation, `if ignore != nil { if matchesIgnoreMatchers(item, ignore.Watch) { acc.suppressWatch = true }; if matchesIgnoreMatchers(item, ignore.Cleanup) { acc.suppressCleanup = true } }`. OR-aggregation: any matching message suppresses the cluster for that rule type. `finalizeClusters` maps booleans to `Suppressed` slice (watch first, cleanup second, omitted when empty). `--no-ignore` sets `ignore = nil` so no annotation is produced.

- [ ] **Step 1: Write the failing unit tests**

Append to `cli/analyze_ignore_test.go`:

```go
func TestBuildAnalyzeReportWatchSuppressed(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	data := []imap.MailData{
		{
			SenderDomains: []string{"github.com"},
			From:          []string{"noreply@github.com"},
			SubjectRaw:    "Hello",
			MessageDate:   time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
		},
	}
	ignore := &appconfig.IgnoreConfig{
		Watch: appconfig.IgnoreMatchers{SenderDomains: []string{"github.com"}},
	}
	report, err := buildAnalyzeReport(data, analyzeReportParams{
		Mailbox:   "INBOX",
		Account:   "user@example.com",
		Generated: now,
		Options: analyzeOptions{
			Top:      100,
			Examples: 20,
			MinCount: 1,
		},
		Ignore: ignore,
	})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	clusters := report.Indexes.SenderLens.Clusters
	if len(clusters) != 1 {
		t.Fatalf("expected 1 sender_unsub_lens cluster, got %d", len(clusters))
	}
	if len(clusters[0].Suppressed) != 1 || clusters[0].Suppressed[0] != "watch" {
		t.Fatalf("expected Suppressed=[watch], got %v", clusters[0].Suppressed)
	}
}

func TestBuildAnalyzeReportBothSuppressed(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	data := []imap.MailData{
		{
			SenderDomains: []string{"github.com"},
			From:          []string{"noreply@github.com"},
			ListID:        "<announce.github.com>",
			SubjectRaw:    "Hello",
			MessageDate:   time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
		},
	}
	ignore := &appconfig.IgnoreConfig{
		Watch:   appconfig.IgnoreMatchers{SenderDomains: []string{"github.com"}},
		Cleanup: appconfig.IgnoreMatchers{ListIDs: []string{"announce.github"}},
	}
	report, err := buildAnalyzeReport(data, analyzeReportParams{
		Mailbox:   "INBOX",
		Account:   "user@example.com",
		Generated: now,
		Options: analyzeOptions{
			Top:      100,
			Examples: 20,
			MinCount: 1,
		},
		Ignore: ignore,
	})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	// The message matches both watch (github.com domain) and cleanup (announce.github list-id).
	// In the normal flow, filterFullyDecided would remove it. Here the cluster is built
	// but annotated with both suppression flags.
	listClusters := report.Indexes.ListLens.Clusters
	if len(listClusters) != 1 {
		t.Fatalf("expected 1 list_lens cluster, got %d", len(listClusters))
	}
	if len(listClusters[0].Suppressed) != 2 ||
		listClusters[0].Suppressed[0] != "watch" ||
		listClusters[0].Suppressed[1] != "cleanup" {
		t.Fatalf("expected Suppressed=[watch,cleanup], got %v", listClusters[0].Suppressed)
	}
}

func TestBuildAnalyzeReportNilIgnoreNoSuppressed(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	data := []imap.MailData{
		{
			SenderDomains: []string{"github.com"},
			From:          []string{"noreply@github.com"},
			SubjectRaw:    "Hello",
			MessageDate:   time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
		},
	}
	report, err := buildAnalyzeReport(data, analyzeReportParams{
		Mailbox:   "INBOX",
		Account:   "user@example.com",
		Generated: now,
		Options: analyzeOptions{
			Top:      100,
			Examples: 20,
			MinCount: 1,
		},
		Ignore: nil,
	})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(payload), "suppressed") {
		t.Fatal("report must not contain 'suppressed' field when Ignore is nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cli/ -run "TestBuildAnalyzeReport" -v`
Expected: FAIL — compile errors (`analyzeReportParams has no field Ignore`, `clusterAccumulator has no field suppressWatch`, etc.).

- [ ] **Step 3: Write the implementation**

In `cli/analyze.go`:

1. Add `Suppressed` field to `analyzeCluster`:

```go
type analyzeCluster struct {
	ClusterID  string                 `json:"cluster_id"`
	Count      int                    `json:"count"`
	LatestDate time.Time              `json:"latest_date"`
	Keys       map[string]any         `json:"keys"`
	Signals    analyzeClusterSignals  `json:"signals"`
	Examples   analyzeClusterExamples `json:"examples"`
	Suppressed []string               `json:"suppressed,omitempty"`
}
```

2. Add `suppressWatch` and `suppressCleanup` to `clusterAccumulator`:

```go
type clusterAccumulator struct {
	count          int
	keys           map[string]any
	hasListID      bool
	hasUnsubscribe bool
	precedence     map[string]int
	latestDate     time.Time
	examples       analyzeClusterExamples
	exampleSets    map[string]map[string]struct{}
	suppressWatch  bool
	suppressCleanup bool
}
```

3. Add `Ignore` to `analyzeReportParams`:

```go
type analyzeReportParams struct {
	Mailbox   string
	Account   string
	Generated time.Time
	AgeWindow *appconfig.AgeWindow
	Options   analyzeOptions
	Ignore    *appconfig.IgnoreConfig
}
```

4. Update `accumulateCluster` — add `ignore` param and per-message check:

```go
func accumulateCluster(acc *clusterAccumulator, item imap.MailData, hasListID bool, maxExamples int, ignore *appconfig.IgnoreConfig) {
	acc.count++
	if !hasListID {
		acc.hasListID = false
	}
	if !item.ListUnsubscribe {
		acc.hasUnsubscribe = false
	}
	if !item.MessageDate.IsZero() && (acc.latestDate.IsZero() || item.MessageDate.After(acc.latestDate)) {
		acc.latestDate = item.MessageDate
	}

	precedence := normalizePrecedenceCategory(item.PrecedenceCategory)
	acc.precedence[precedence]++

	if ignore != nil {
		if matchesIgnoreMatchers(item, ignore.Watch) {
			acc.suppressWatch = true
		}
		if matchesIgnoreMatchers(item, ignore.Cleanup) {
			acc.suppressCleanup = true
		}
	}

	addExample(acc, ExampleKeySubjectRaw, strings.TrimSpace(item.SubjectRaw), maxExamples)
	for _, recipient := range item.Recipients {
		addExample(acc, ExampleKeyRecipients, recipient, maxExamples)
	}
	for _, replyTo := range item.ReplyToDomains {
		addExample(acc, ExampleKeyReplyToDomains, replyTo, maxExamples)
	}
	for _, senderDomain := range item.SenderDomains {
		addExample(acc, ExampleKeySenderDomains, senderDomain, maxExamples)
	}
	if strings.TrimSpace(item.ReturnPathDomain) != "" {
		addExample(acc, ExampleKeyReturnPathDomains, item.ReturnPathDomain, maxExamples)
	}
	for _, target := range splitAndTrim(item.ListUnsubscribeTargets) {
		addExample(acc, ExampleKeyListUnsubscribeTargets, target, maxExamples)
	}
}
```

5. Update all four lens builders — add `ignore` param and pass through to `accumulateCluster`:

```go
func buildListLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		listID := normalizeListID(item.ListID)
		if listID == "" {
			continue
		}
		keyString := fmt.Sprintf("ListID=%s", listID)
		clusterID := makeClusterID("list_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"ListID": listID,
		})
		accumulateCluster(acc, item, true, options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"ListID"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

func buildSenderUnsubLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		senderDomains := normalizeDomains(item.SenderDomains)
		if len(senderDomains) == 1 && strings.TrimSpace(senderDomains[0]) == "" {
			continue
		}
		hasUnsub := item.ListUnsubscribe
		keyString := fmt.Sprintf("SenderDomains=%s|HasListUnsubscribe=%s", strings.Join(senderDomains, ","), boolString(hasUnsub))
		clusterID := makeClusterID("sender_unsub_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"SenderDomains":      senderDomains,
			"HasListUnsubscribe": hasUnsub,
			"FromList":           item.From,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"SenderDomains", "HasListUnsubscribe"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

func buildTemplateLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		senderDomains := normalizeDomains(item.SenderDomains)
		subject := strings.TrimSpace(item.SubjectNormalized)
		keyString := fmt.Sprintf("SenderDomains=%s|SubjectNormalized=%s", strings.Join(senderDomains, ","), subject)
		clusterID := makeClusterID("template_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"SenderDomains":     senderDomains,
			"SubjectNormalized": subject,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"SenderDomains", "SubjectNormalized"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

func buildRecipientTagLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		tags := normalizeRecipientTags(item.RecipientTags)
		if len(tags) == 0 {
			continue
		}
		joined := strings.Join(tags, ",")
		keyString := fmt.Sprintf("recipient_tag=%s", joined)
		clusterID := makeClusterID("recipient_tag_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"recipient_tag": joined,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"recipient_tag"},
		Clusters:  finalizeClusters(clusters, options),
	}
}
```

6. Update `finalizeClusters` — map booleans to `Suppressed` slice:

```go
func finalizeClusters(clusters map[string]*clusterAccumulator, options analyzeOptions) []analyzeCluster {
	minCount := options.MinCount
	if minCount <= 0 {
		minCount = 1
	}
	all := make([]analyzeCluster, 0, len(clusters))
	for clusterID, acc := range clusters {
		if acc.count < minCount {
			continue
		}
		suppressed := make([]string, 0, 2)
		if acc.suppressWatch {
			suppressed = append(suppressed, "watch")
		}
		if acc.suppressCleanup {
			suppressed = append(suppressed, "cleanup")
		}
		all = append(all, analyzeCluster{
			ClusterID:  clusterID,
			Count:      acc.count,
			LatestDate: acc.latestDate,
			Keys:       acc.keys,
			Signals: analyzeClusterSignals{
				HasListID:            acc.hasListID,
				HasListUnsubscribe:   acc.hasUnsubscribe,
				PrecedenceCategories: acc.precedence,
			},
			Examples:   acc.examples,
			Suppressed: suppressed,
		})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].ClusterID < all[j].ClusterID
	})
	if options.Top > 0 && len(all) > options.Top {
		return all[:options.Top]
	}
	return all
}
```

7. Update `buildAnalyzeReport` — pass `params.Ignore` to builders:

Replace the four builder calls:

```go
	listLens := buildListLens(data, options)
	senderLens := buildSenderUnsubLens(data, options)
	templateLens := buildTemplateLens(data, options)
	recipientTagLens := buildRecipientTagLens(data, options)
```

with:

```go
	listLens := buildListLens(data, options, params.Ignore)
	senderLens := buildSenderUnsubLens(data, options, params.Ignore)
	templateLens := buildTemplateLens(data, options, params.Ignore)
	recipientTagLens := buildRecipientTagLens(data, options, params.Ignore)
```

8. Update RunE — compute `ignore` once and pass via params:

After the `noIgnore` flag read (from Task 3), add:

```go
		ignore := cfg.Ignore
		if noIgnore {
			ignore = nil
		}
```

And in the `buildAnalyzeReport` call, add `Ignore: ignore` to the params:

```go
			report, err := buildAnalyzeReport(data, analyzeReportParams{
				Mailbox:   mailbox,
				Account:   imapEnv.User,
				Generated: time.Now().UTC(),
				AgeWindow: rule.Server.AgeWindow,
				Options:   options,
				Ignore:    ignore,
			})
```

- [ ] **Step 4: Run unit tests to verify they pass**

Run: `go test ./cli/ -run "TestBuildAnalyzeReport" -v`
Expected: PASS (3 new unit tests + the existing `TestBuildAnalyzeReportJSON` still passes).

- [ ] **Step 5: Add e2e tests for the suppressed annotation**

Append to `cli/analyze_ignore_e2e_test.go`:

```go
func TestAnalyzeAnnotatesWatchSuppressedCluster(t *testing.T) {
	report := runAnalyzeToReport(t)

	indexes, ok := report["indexes"].(map[string]any)
	if !ok {
		t.Fatal("report missing indexes")
	}
	senderLens, ok := indexes["sender_unsub_lens"].(map[string]any)
	if !ok {
		t.Fatal("report missing sender_unsub_lens")
	}
	clusters, ok := senderLens["clusters"].([]any)
	if !ok {
		t.Fatal("sender_unsub_lens.clusters is invalid")
	}

	var found bool
	for _, c := range clusters {
		cluster := c.(map[string]any)
		keys := cluster["keys"].(map[string]any)
		domains, ok := keys["SenderDomains"].([]any)
		if ok && len(domains) == 1 && domains[0] == "github.com" {
			suppressed, ok := cluster["suppressed"].([]any)
			if !ok || len(suppressed) != 1 || suppressed[0] != "watch" {
				t.Fatalf("expected suppressed=[watch], got %v", cluster["suppressed"])
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected to find github.com cluster in sender_unsub_lens")
	}
}

func TestAnalyzeNoIgnoreNoSuppressedField(t *testing.T) {
	report := runAnalyzeToReport(t, "--no-ignore")

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "suppressed") {
		t.Fatal("report with --no-ignore must not contain 'suppressed' field")
	}
}
```

- [ ] **Step 6: Run all tests to verify they pass**

Run: `go test ./cli/ -v`
Expected: all tests pass. Then `go test ./...` — full suite green. `gofmt -l cli/` prints nothing, `go vet ./cli/` clean.

- [ ] **Step 7: Commit**

```bash
git add cli/analyze.go cli/analyze_ignore_test.go cli/analyze_ignore_e2e_test.go
git commit -m "feat(cli): annotate clusters with per-rule-type suppression"
```

---

### Task 5: generator — reads suppression annotation from report (TDD)

**Files:**
- Modify: `bin/postmanpat-generate-rules.py` (process_cluster reads `suppressed`; main loop skips both-suppressed)
- Test: `bin/test_generate_rules.py` (add `TestProcessClusterSuppression`)

**Interfaces:**
- Consumes: `process_cluster` (existing), `prompt_yes_no` (existing).
- Produces: `process_cluster` reads `cluster.get("suppressed", [])`; if `"watch" in suppressed` → print note and skip watch prompt; if `"cleanup" in suppressed` → print note and skip cleanup prompt. Main loop: if both in suppressed → increment counter, `continue` without prompting/checkpointing; after loop print summary when N>0.

- [ ] **Step 1: Write the failing tests**

Add to `bin/test_generate_rules.py` (add `from unittest import mock` and `from io import StringIO` to imports, and `import contextlib`):

```python
class TestProcessClusterSuppression(unittest.TestCase):
    def _cluster(self, suppressed=None):
        return {
            "cluster_id": "list_lens:abc",
            "count": 5,
            "latest_date": "2026-08-01T00:00:00Z",
            "keys": {"ListID": "some.list"},
            "signals": {"has_list_id": True, "has_list_unsubscribe": True, "precedence_categories": {}},
            "examples": {"subject_raw": ["Hello"], "recipients": [], "reply_to_domains": [],
                         "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []},
            "suppressed": suppressed or [],
        }

    def test_watch_suppressed_skips_watch_prompt(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        buf = StringIO()
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with contextlib.redirect_stdout(buf):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(suppressed=["watch"]),
                    watch_rules, cleanup_rules, "INBOX",
                )
        self.assertTrue(proceed)
        self.assertNotIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)
        self.assertEqual(watch_rules, [])
        output = buf.getvalue()
        self.assertIn("Watch rule suppressed by ignore list.", output)

    def test_cleanup_suppressed_skips_cleanup_prompt(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        buf = StringIO()
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with contextlib.redirect_stdout(buf):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(suppressed=["cleanup"]),
                    watch_rules, cleanup_rules, "INBOX",
                )
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertNotIn("Generate cleanup rule?", asked)
        self.assertEqual(cleanup_rules, [])
        output = buf.getvalue()
        self.assertIn("Cleanup rule suppressed by ignore list.", output)

    def test_both_suppressed_skips_both_prompts(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        buf = StringIO()
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with contextlib.redirect_stdout(buf):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(suppressed=["watch", "cleanup"]),
                    watch_rules, cleanup_rules, "INBOX",
                )
        self.assertTrue(proceed)
        self.assertNotIn("Generate watch rule?", asked)
        self.assertNotIn("Generate cleanup rule?", asked)
        output = buf.getvalue()
        self.assertIn("Watch rule suppressed by ignore list.", output)
        self.assertIn("Cleanup rule suppressed by ignore list.", output)

    def test_no_suppression_asks_both(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            proceed, _ = mod.process_cluster(
                "list_lens", self._cluster(suppressed=[]),
                watch_rules, cleanup_rules, "INBOX",
            )
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)

    def test_no_suppressed_field_asks_both(self):
        asked = []
        def fake_prompt(message, default=False, allow_ignore=False):
            asked.append(message)
            return "n"

        cluster = self._cluster()
        del cluster["suppressed"]
        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            proceed, _ = mod.process_cluster(
                "list_lens", cluster,
                watch_rules, cleanup_rules, "INBOX",
            )
        self.assertTrue(proceed)
        self.assertIn("Generate watch rule?", asked)
        self.assertIn("Generate cleanup rule?", asked)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 bin/test_generate_rules.py -k TestProcessClusterSuppression -v`
Expected: FAIL — assertions fail because `process_cluster` doesn't read `suppressed` yet.

- [ ] **Step 3: Write the implementation**

Replace the entire `process_cluster` function in `bin/postmanpat-generate-rules.py` with:

```python
def process_cluster(
    lens: str,
    cluster: Dict[str, Any],
    watch_rules: List[Dict[str, Any]],
    cleanup_rules: List[Dict[str, Any]],
    default_folders: Optional[str],
) -> Tuple[bool, Optional[str]]:
    print("\n=== Cluster ===")
    print(cluster_summary(cluster))
    for line in format_examples(cluster):
        print(f"  {line}")

    suppressed = cluster.get("suppressed", []) or []
    rule_name: Optional[str] = None

    if "watch" in suppressed:
        print("Watch rule suppressed by ignore list.")
    else:
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

    if "cleanup" in suppressed:
        print("Cleanup rule suppressed by ignore list.")
    else:
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

    return True, default_folders
```

Replace the main cluster loop in `main()` (the `for lens, cluster in clusters:` block) with:

```python
    suppressed_count = 0

    for lens, cluster in clusters:
        if not enabled_lenses.get(lens, True):
            continue
        cluster_id = str(cluster.get("cluster_id", ""))
        if cluster_id in processed_ids:
            continue

        suppressed = cluster.get("suppressed", []) or []
        if "watch" in suppressed and "cleanup" in suppressed:
            suppressed_count += 1
            continue  # not prompted -> not written to the Generation Checkpoint

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

    if suppressed_count:
        print(f"Skipped {suppressed_count} clusters suppressed by ignore list")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python3 bin/test_generate_rules.py -v`
Expected: all tests pass (Task 5's 5 + any pre-existing). Also `python3 bin/test_yaml_scalar.py` still passes.

- [ ] **Step 5: Manual verification of both-suppressed skip**

```bash
cat > /tmp/pp-analyze-suppressed.json << 'EOF'
{
  "generated_at": "2026-08-06T00:00:00Z",
  "source": {"mailbox": "INBOX", "account": "test@test.com", "time_window": {"after": "", "before": ""}},
  "stats": {"total_messages_scanned": 10},
  "indexes": {
    "list_lens": {
      "key_fields": ["ListID"],
      "clusters": [
        {"cluster_id": "list_lens:both", "count": 5, "latest_date": "2026-08-01T00:00:00Z", "keys": {"ListID": "ignored.both"}, "signals": {"has_list_id": true, "has_list_unsubscribe": true, "precedence_categories": {}}, "examples": {"subject_raw": ["News"], "recipients": [], "reply_to_domains": [], "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []}, "suppressed": ["watch", "cleanup"]},
        {"cluster_id": "list_lens:active", "count": 3, "latest_date": "2026-08-01T00:00:00Z", "keys": {"ListID": "active.sender"}, "signals": {"has_list_id": true, "has_list_unsubscribe": true, "precedence_categories": {}}, "examples": {"subject_raw": ["Hi"], "recipients": [], "reply_to_domains": [], "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []}}
      ]
    },
    "sender_unsub_lens": {"key_fields": ["SenderDomains", "HasListUnsubscribe"], "clusters": []},
    "template_lens": {"key_fields": ["SenderDomains", "SubjectNormalized"], "clusters": []},
    "recipient_tag_lens": {"key_fields": ["recipient_tag"], "clusters": []}
  }
}
EOF

printf 'y\nn\nn\nn\n' | python3 bin/postmanpat-generate-rules.py --analyze /tmp/pp-analyze-suppressed.json --watch-out /tmp/pp-watch.yml --cleanup-out /tmp/pp-cleanup.yml
```

Expected: "Skipped 1 clusters suppressed by ignore list" prints; only `active.sender` is prompted; checkpoint contains only `list_lens:active`.

- [ ] **Step 6: Commit**

```bash
git add bin/postmanpat-generate-rules.py bin/test_generate_rules.py
git commit -m "feat(bin): read suppression annotation from analyze report"
```

---

### Task 6: generator — authoring ("i" + scope follow-up + --ignore-out) (TDD)

**Files:**
- Modify: `bin/postmanpat-generate-rules.py` (prompt_yes_no extension, new pure functions, process_cluster authoring, argparse --ignore-out, end-of-main write/warning)
- Test: `bin/test_generate_rules.py` (add `TestIgnorePureFunctions`, `TestProcessClusterAuthoring`)

**Interfaces:**
- Consumes: existing `prompt_yes_no`, `process_cluster`, `write_yaml`.
- Produces: `prompt_yes_no` gains `allow_ignore` param (hint `[y/n/i/q]`, returns `"i"`); new pure functions `extract_ignore_identity`, `merge_ignore_entries`, `dedup_ignore_entries`, `build_ignore_fragment`; `process_cluster` gains `ignore_watch`/`ignore_cleanup` accumulator params; `--ignore-out` CLI arg; end-of-main writes fragment or prints warning.

- [ ] **Step 1: Write the failing tests**

Add to `bin/test_generate_rules.py`:

```python
class TestIgnorePureFunctions(unittest.TestCase):
    def test_extract_list_lens(self):
        cluster = {"keys": {"ListID": "github.notifications"}}
        result = mod.extract_ignore_identity("list_lens", cluster)
        self.assertEqual(result, {"list_ids": ["github.notifications"]})

    def test_extract_sender_unsub_single_domain(self):
        cluster = {"keys": {"SenderDomains": ["github.com"]}}
        result = mod.extract_ignore_identity("sender_unsub_lens", cluster)
        self.assertEqual(result, {"sender_domains": ["github.com"]})

    def test_extract_sender_unsub_multi_domain(self):
        cluster = {"keys": {"SenderDomains": ["github.com", "actions.githubusercontent.com"]}}
        result = mod.extract_ignore_identity("sender_unsub_lens", cluster)
        self.assertEqual(set(result["sender_domains"]), {"github.com", "actions.githubusercontent.com"})
        self.assertEqual(len(result["sender_domains"]), 2)

    def test_extract_recipient_tag(self):
        cluster = {"keys": {"recipient_tag": "newsletter,weekly"}}
        result = mod.extract_ignore_identity("recipient_tag_lens", cluster)
        self.assertEqual(result, {"recipient_tags": ["newsletter,weekly"]})

    def test_extract_unknown_lens(self):
        cluster = {"keys": {"ListID": "x"}}
        result = mod.extract_ignore_identity("template_lens", cluster)
        self.assertEqual(result, {})

    def test_extract_missing_key(self):
        cluster = {"keys": {}}
        result = mod.extract_ignore_identity("list_lens", cluster)
        self.assertEqual(result, {})

    def test_merge_entries(self):
        acc = {"list_ids": ["a"]}
        mod.merge_ignore_entries(acc, {"list_ids": ["b"], "sender_domains": ["x.com"]})
        self.assertEqual(acc["list_ids"], ["a", "b"])
        self.assertEqual(acc["sender_domains"], ["x.com"])

    def test_merge_into_empty(self):
        acc = {}
        mod.merge_ignore_entries(acc, {"list_ids": ["a"]})
        self.assertEqual(acc, {"list_ids": ["a"]})

    def test_dedup_entries(self):
        entries = {"list_ids": ["b", "a", "b"], "sender_domains": []}
        result = mod.dedup_ignore_entries(entries)
        self.assertEqual(result, {"list_ids": ["a", "b"]})

    def test_dedup_drops_empty(self):
        entries = {"list_ids": ["a"], "sender_domains": []}
        result = mod.dedup_ignore_entries(entries)
        self.assertEqual(result, {"list_ids": ["a"]})
        self.assertNotIn("sender_domains", result)

    def test_build_ignore_fragment_empty(self):
        result = mod.build_ignore_fragment({}, {})
        self.assertEqual(result, {})

    def test_build_ignore_fragment_watch_only(self):
        result = mod.build_ignore_fragment({"list_ids": ["a"]}, {})
        self.assertEqual(result, {"ignore": {"watch": {"list_ids": ["a"]}}})

    def test_build_ignore_fragment_both(self):
        result = mod.build_ignore_fragment(
            {"list_ids": ["a"]},
            {"sender_domains": ["x.com"]},
        )
        self.assertIn("watch", result["ignore"])
        self.assertIn("cleanup", result["ignore"])
```

```python
class TestProcessClusterAuthoring(unittest.TestCase):
    def _cluster(self, suppressed=None):
        return {
            "cluster_id": "list_lens:abc",
            "count": 5,
            "latest_date": "2026-08-01T00:00:00Z",
            "keys": {"ListID": "some.list"},
            "signals": {"has_list_id": True, "has_list_unsubscribe": True, "precedence_categories": {}},
            "examples": {"subject_raw": ["Hello"], "recipients": [], "reply_to_domains": [],
                         "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []},
            "suppressed": suppressed or [],
        }

    def test_watch_i_records_to_watch_accumulator(self):
        ignore_watch = {}
        ignore_cleanup = {}
        responses = iter(["i", "n", "n"])
        def fake_prompt(message, default=False, allow_ignore=False):
            return next(responses)

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with mock.patch.object(mod, "_prompt_yes_no_simple", return_value=False):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(),
                    watch_rules, cleanup_rules, "INBOX",
                    ignore_watch=ignore_watch,
                    ignore_cleanup=ignore_cleanup,
                )
        self.assertTrue(proceed)
        self.assertEqual(ignore_watch, {"list_ids": ["some.list"]})
        self.assertEqual(ignore_cleanup, {})

    def test_watch_i_followup_y_records_to_both_skips_cleanup_prompt(self):
        ignore_watch = {}
        ignore_cleanup = {}
        responses = iter(["i"])
        def fake_prompt(message, default=False, allow_ignore=False):
            return next(responses)

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            with mock.patch.object(mod, "_prompt_yes_no_simple", return_value=True):
                proceed, _ = mod.process_cluster(
                    "list_lens", self._cluster(),
                    watch_rules, cleanup_rules, "INBOX",
                    ignore_watch=ignore_watch,
                    ignore_cleanup=ignore_cleanup,
                )
        self.assertTrue(proceed)
        self.assertEqual(ignore_watch, {"list_ids": ["some.list"]})
        self.assertEqual(ignore_cleanup, {"list_ids": ["some.list"]})
        self.assertEqual(watch_rules, [])
        self.assertEqual(cleanup_rules, [])

    def test_cleanup_i_records_to_cleanup_only(self):
        ignore_watch = {}
        ignore_cleanup = {}
        responses = iter(["n", "i"])
        def fake_prompt(message, default=False, allow_ignore=False):
            return next(responses)

        watch_rules = []
        cleanup_rules = []
        with mock.patch.object(mod, "prompt_yes_no", side_effect=fake_prompt):
            proceed, _ = mod.process_cluster(
                "list_lens", self._cluster(),
                watch_rules, cleanup_rules, "INBOX",
                ignore_watch=ignore_watch,
                ignore_cleanup=ignore_cleanup,
            )
        self.assertTrue(proceed)
        self.assertEqual(ignore_watch, {})
        self.assertEqual(ignore_cleanup, {"list_ids": ["some.list"]})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python3 bin/test_generate_rules.py -k "TestIgnorePureFunctions|TestProcessClusterAuthoring" -v`
Expected: FAIL — `AttributeError` for missing functions.

- [ ] **Step 3: Write the implementation**

1. Extend `prompt_yes_no` — replace the existing function:

```python
def prompt_yes_no(message: str, default: bool = False, allow_ignore: bool = False) -> str:
    if allow_ignore:
        default_hint = "y" if default else "n"
        response = input(f"{message} [y/n/i/q] (default {default_hint}): ").strip().lower()
    else:
        default_hint = "y" if default else "n"
        response = input(f"{message} [y/n/q] (default {default_hint}): ").strip().lower()
    if response == "":
        return "y" if default else "n"
    if response in ("q", "quit"):
        return "q"
    if allow_ignore and response == "i":
        return "i"
    if response not in ("y", "n"):
        return "n"
    return response
```

2. Add a simple follow-up helper (placed after `prompt_yes_no`):

```python
def _prompt_yes_no_simple(message: str, default: bool = False) -> bool:
    """y/n prompt without quit/ignore options. Returns True for yes."""
    hint = "y" if default else "n"
    response = input(f"{message} [y/n] (default {hint}): ").strip().lower()
    if response == "":
        return default
    return response == "y"
```

3. Add the pure functions (placed after `_prompt_yes_no_simple`):

```python
def extract_ignore_identity(lens: str, cluster: Dict[str, Any]) -> Dict[str, List[str]]:
    """Extract the ignore-relevant identity from a cluster for a given lens.

    list_lens -> {"list_ids": [keys.ListID]}
    sender_unsub_lens -> {"sender_domains": [all of keys.SenderDomains]}
    recipient_tag_lens -> {"recipient_tags": [keys["recipient_tag"]]}
    else -> {}.
    """
    keys = cluster.get("keys", {}) or {}
    if lens == "list_lens":
        list_id = keys.get("ListID")
        if list_id:
            return {"list_ids": [str(list_id)]}
    elif lens == "sender_unsub_lens":
        domains = keys.get("SenderDomains")
        if domains:
            return {"sender_domains": [str(d) for d in domains]}
    elif lens == "recipient_tag_lens":
        tag = keys.get("recipient_tag")
        if tag:
            return {"recipient_tags": [str(tag)]}
    return {}


def merge_ignore_entries(acc: Dict[str, List[str]], new: Dict[str, List[str]]) -> None:
    """Append new values into acc lists (in place)."""
    for key, values in new.items():
        if key not in acc:
            acc[key] = []
        acc[key].extend(values)


def dedup_ignore_entries(entries: Dict[str, List[str]]) -> Dict[str, List[str]]:
    """sorted(set(v)) per field, empty fields dropped."""
    result = {}
    for key, values in entries.items():
        deduped = sorted(set(values))
        if deduped:
            result[key] = deduped
    return result


def build_ignore_fragment(ignore_watch: Dict[str, List[str]], ignore_cleanup: Dict[str, List[str]]) -> Dict[str, Any]:
    """{"ignore": {"watch": deduped, "cleanup": deduped}} omitting empty sides; {} when both empty."""
    watch = dedup_ignore_entries(ignore_watch)
    cleanup = dedup_ignore_entries(ignore_cleanup)
    result = {}
    if watch:
        result["watch"] = watch
    if cleanup:
        result["cleanup"] = cleanup
    if result:
        return {"ignore": result}
    return {}
```

4. Replace `process_cluster` with the full authoring version:

```python
def process_cluster(
    lens: str,
    cluster: Dict[str, Any],
    watch_rules: List[Dict[str, Any]],
    cleanup_rules: List[Dict[str, Any]],
    default_folders: Optional[str],
    ignore_watch: Optional[Dict[str, List[str]]] = None,
    ignore_cleanup: Optional[Dict[str, List[str]]] = None,
) -> Tuple[bool, Optional[str]]:
    print("\n=== Cluster ===")
    print(cluster_summary(cluster))
    for line in format_examples(cluster):
        print(f"  {line}")

    suppressed = cluster.get("suppressed", []) or []
    rule_name: Optional[str] = None
    skip_cleanup_prompt = False

    # Watch section
    if "watch" in suppressed:
        print("Watch rule suppressed by ignore list.")
    else:
        watch_response = prompt_yes_no("Generate watch rule?", default=False, allow_ignore=True)
        if watch_response == "q":
            return False, default_folders
        if watch_response == "i":
            if ignore_watch is not None:
                merge_ignore_entries(ignore_watch, extract_ignore_identity(lens, cluster))
            if _prompt_yes_no_simple("Also ignore for cleanup?", default=False):
                if ignore_cleanup is not None:
                    merge_ignore_entries(ignore_cleanup, extract_ignore_identity(lens, cluster))
                skip_cleanup_prompt = True
        elif watch_response == "y":
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

    # Cleanup section
    if not skip_cleanup_prompt:
        if "cleanup" in suppressed:
            print("Cleanup rule suppressed by ignore list.")
        else:
            cleanup_response = prompt_yes_no("Generate cleanup rule?", default=False, allow_ignore=True)
            if cleanup_response == "q":
                return False, default_folders
            if cleanup_response == "i":
                if ignore_cleanup is not None:
                    merge_ignore_entries(ignore_cleanup, extract_ignore_identity(lens, cluster))
            elif cleanup_response == "y":
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

    return True, default_folders
```

5. Add `--ignore-out` arg in `main()`, after the `--checkpoint` block:

```python
    parser.add_argument(
        "--ignore-out",
        default=None,
        help="Path to write authored ignore entries as a YAML fragment",
    )
```

6. Replace the main cluster loop (extending Task 5's loop with ignore accumulators):

```python
    suppressed_count = 0
    ignore_watch_acc: Dict[str, List[str]] = {}
    ignore_cleanup_acc: Dict[str, List[str]] = {}

    for lens, cluster in clusters:
        if not enabled_lenses.get(lens, True):
            continue
        cluster_id = str(cluster.get("cluster_id", ""))
        if cluster_id in processed_ids:
            continue

        suppressed = cluster.get("suppressed", []) or []
        if "watch" in suppressed and "cleanup" in suppressed:
            suppressed_count += 1
            continue

        proceed, default_folders = process_cluster(
            lens,
            cluster,
            watch_rules,
            cleanup_rules,
            default_folders,
            ignore_watch=ignore_watch_acc,
            ignore_cleanup=ignore_cleanup_acc,
        )
        if not proceed:
            break
        processed_ids.add(cluster_id)
        save_checkpoint(checkpoint_path, processed_ids)

    if suppressed_count:
        print(f"Skipped {suppressed_count} clusters suppressed by ignore list")
```

7. Add end-of-main write/warning after the existing `write_yaml` / checkpoint print lines (before `return 0`):

```python
    fragment = build_ignore_fragment(ignore_watch_acc, ignore_cleanup_acc)
    if fragment:
        if args.ignore_out:
            write_yaml(args.ignore_out, fragment)
            print(f"Wrote ignore fragment to {args.ignore_out}")
        else:
            entry_count = sum(len(v) for v in fragment.get("ignore", {}).get("watch", {}).values())
            entry_count += sum(len(v) for v in fragment.get("ignore", {}).get("cleanup", {}).values())
            print(f"Warning: {entry_count} ignore entries were authored but not written. Use --ignore-out to save them.")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python3 bin/test_generate_rules.py -v`
Expected: all tests pass (pure functions + authoring + suppression + pre-existing). Also `python3 bin/test_yaml_scalar.py` still passes.

- [ ] **Step 5: Manual verification of the interactive authoring flow**

```bash
cat > /tmp/pp-analyze-author.json << 'EOF'
{
  "generated_at": "2026-08-06T00:00:00Z",
  "source": {"mailbox": "INBOX", "account": "test@test.com", "time_window": {"after": "", "before": ""}},
  "stats": {"total_messages_scanned": 5},
  "indexes": {
    "list_lens": {
      "key_fields": ["ListID"],
      "clusters": [
        {"cluster_id": "list_lens:aaa", "count": 5, "latest_date": "2026-08-01T00:00:00Z", "keys": {"ListID": "ignore-me.list"}, "signals": {"has_list_id": true, "has_list_unsubscribe": true, "precedence_categories": {}}, "examples": {"subject_raw": ["News"], "recipients": [], "reply_to_domains": [], "sender_domains": [], "returnpath_domains": [], "list_unsubscribe_targets": []}}
      ]
    },
    "sender_unsub_lens": {"key_fields": ["SenderDomains", "HasListUnsubscribe"], "clusters": []},
    "template_lens": {"key_fields": ["SenderDomains", "SubjectNormalized"], "clusters": []},
    "recipient_tag_lens": {"key_fields": ["recipient_tag"], "clusters": []}
  }
}
EOF

# Answer: process list_lens (y), watch prompt "i" (ignore), follow-up "n" (cleanup only), cleanup prompt "n"
printf 'y\ni\nn\nn\n' | python3 bin/postmanpat-generate-rules.py --analyze /tmp/pp-analyze-author.json --watch-out /tmp/pp-watch.yml --cleanup-out /tmp/pp-cleanup.yml --ignore-out /tmp/pp-ignore.yaml
```

Expected: no watch/cleanup rules generated; `/tmp/pp-ignore.yaml` contains:
```yaml
ignore:
  watch:
    list_ids:
      - "ignore-me.list"
```
Checkpoint contains `list_lens:aaa`.

Now test the follow-up "y" path (skip cleanup prompt):
```bash
printf 'y\ni\ny\n' | python3 bin/postmanpat-generate-rules.py --analyze /tmp/pp-analyze-author.json --watch-out /tmp/pp-watch2.yml --cleanup-out /tmp/pp-cleanup2.yml --ignore-out /tmp/pp-ignore2.yaml
```

Expected: `/tmp/pp-ignore2.yaml` contains both watch and cleanup entries with the same list_id.

- [ ] **Step 6: Commit**

```bash
git add bin/postmanpat-generate-rules.py bin/test_generate_rules.py
git commit -m "feat(bin): add ignore authoring via 'i' prompt and --ignore-out"
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
    - `--no-ignore`: disable ignore-list filtering and suppression annotation for this run (audit what you're ignoring).
    - Optional top-level `ignore` section filters Fully Decided mail out of the report. Identities on both the `watch` and `cleanup` lists are removed before aggregation; identities on one list stay in the report and the rule generator suppresses only that rule type's prompt. Each cluster in the report carries a `"suppressed"` annotation (e.g. `"suppressed": ["watch"]`) so the generator knows which prompts to skip. `sender_domains` match exactly; all other fields are case-insensitive substrings (`subject_substrings` match raw subjects):

      ```yaml
      ignore:
        watch:
          sender_domains: ["github.com"]
        cleanup:
          sender_domains: ["github.com"]
          list_ids: ["weekly-digest"]
      ```
```

In "Turn the report into rules", update the script example:

```markdown
    ```bash
    python3 bin/postmanpat-generate-rules.py \
      --analyze analyze-out/postmanpat-analyze-*.json \
      --watch-out watch-new.yml \
      --cleanup-out cleanup-new.yml \
      --ignore-out ignore-new.yaml  # optional: writes authored "i" entries as a YAML fragment
    ```

    Answer `i` at a rule prompt to ignore that identity instead of generating a rule. The script will ask whether to ignore for the other rule type too. Use `--ignore-out` to persist the authored fragment for review and merge into your config.
```

- [ ] **Step 2: Update AGENTS.md**

In "CLI Behavior", extend the `analyze` bullet's flag list from ``(--top` `--examples` `--min-count`)`` to ``(--top` `--examples` `--min-count` `--no-ignore`)`` and append: "An optional top-level `ignore:` section (`watch:`/`cleanup:` sub-lists) filters Fully Decided messages (on both lists) out of the report; each surviving cluster carries a `suppressed` annotation (`["watch"]`, `["cleanup"]`, or both) that the rule generator uses to skip prompts. See `CONTEXT.md` and `docs/adr/0002-suppression-via-report-annotation.md`."

In "Project Structure", extend the `bin/` line to note `postmanpat-generate-rules.py` accepts `--ignore-out` for authoring ignore entries and reads the report's `suppressed` annotation (no config-side matching).

- [ ] **Step 3: Verify and commit**

Run: `go test ./... && python3 bin/test_generate_rules.py && python3 bin/test_yaml_scalar.py`
Expected: all green (docs-only change; full verification pass).

```bash
git add README.md AGENTS.md
git commit -m "docs: document ignore list, --no-ignore, suppression annotation, and --ignore-out"
```
