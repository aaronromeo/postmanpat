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
