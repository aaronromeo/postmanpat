package searches

import (
	"testing"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/emersion/go-imap/v2"
)

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
