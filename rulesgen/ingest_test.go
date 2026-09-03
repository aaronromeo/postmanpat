package rulesgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reportA = `{
  "generated_at": "2026-09-02T03:30:00Z",
  "source": {"mailbox": "INBOX", "account": "aaron@example.com"},
  "stats": {"total_messages_scanned": 30},
  "indexes": {
    "list_lens": {
      "key_fields": ["ListID"],
      "clusters": [
        {
          "cluster_id": "list-both-suppressed",
          "count": 5,
          "latest_date": "2026-09-01T10:00:00Z",
          "keys": {"ListID": "updates.example.com"},
          "signals": {"has_list_id": true, "has_list_unsubscribe": true, "precedence_categories": {"list": 5}},
          "examples": {"subject_raw": ["Weekly update"], "list_unsubscribe_targets": ["https://example.com/unsub"]},
          "suppressed": ["watch", "cleanup"]
        }
      ]
    },
    "sender_unsub_lens": {
      "key_fields": ["SenderDomains", "HasListUnsubscribe"],
      "clusters": [
        {
          "cluster_id": "sender-news",
          "count": 3,
          "latest_date": "2026-09-01T09:00:00Z",
          "keys": {"SenderDomains": ["news.example.com"], "HasListUnsubscribe": true},
          "signals": {"has_list_unsubscribe": true, "precedence_categories": {"bulk": 3}},
          "examples": {"subject_raw": ["Hello", "Weekly"], "recipients": ["aaron@example.com"], "sender_domains": ["news.example.com"]}
        },
        {
          "cluster_id": "sender-shop",
          "count": 2,
          "latest_date": "2026-09-01T08:00:00Z",
          "keys": {"SenderDomains": ["shop.example.org"], "HasListUnsubscribe": false},
          "signals": {},
          "examples": {"subject_raw": ["Sale"], "sender_domains": ["shop.example.org"]},
          "suppressed": ["cleanup"]
        }
      ]
    },
    "template_lens": {
      "key_fields": ["SubjectNormalized"],
      "clusters": [
        {
          "cluster_id": "template-never-ingested",
          "count": 4,
          "latest_date": "2026-09-01T07:00:00Z",
          "keys": {"SubjectNormalized": "your order has shipped"},
          "signals": {},
          "examples": {"subject_raw": ["Your order has shipped!"]}
        }
      ]
    },
    "recipient_tag_lens": {
      "key_fields": ["recipient_tag"],
      "clusters": [
        {
          "cluster_id": "tag-newsletters",
          "count": 6,
          "latest_date": "2026-09-01T06:00:00Z",
          "keys": {"recipient_tag": "news"},
          "signals": {},
          "examples": {"subject_raw": ["Newsletter 1", "Newsletter 2"]}
        }
      ]
    }
  }
}`

const reportB = `{
  "generated_at": "2026-09-03T03:30:00Z",
  "source": {"mailbox": "INBOX", "account": "aaron@example.com"},
  "stats": {"total_messages_scanned": 45},
  "indexes": {
    "list_lens": {"clusters": []},
    "sender_unsub_lens": {
      "clusters": [
        {
          "cluster_id": "sender-news",
          "count": 9,
          "latest_date": "2026-09-02T09:00:00Z",
          "keys": {"SenderDomains": ["news.example.com"], "HasListUnsubscribe": true},
          "signals": {"has_list_unsubscribe": true, "precedence_categories": {"bulk": 9}},
          "examples": {"subject_raw": ["Hello", "Weekly", "Extra"], "recipients": ["aaron@example.com"], "sender_domains": ["news.example.com"]}
        },
        {
          "cluster_id": "sender-newcomer",
          "count": 1,
          "latest_date": "2026-09-02T12:00:00Z",
          "keys": {"SenderDomains": ["new.example.net"], "HasListUnsubscribe": false},
          "signals": {},
          "examples": {"subject_raw": ["Welcome aboard"]}
        }
      ]
    },
    "template_lens": {"clusters": []},
    "recipient_tag_lens": {"clusters": []}
  }
}`

func writeReport(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func pendingIDs(t *testing.T, st *Store) []string {
	t.Helper()
	pending, err := st.PendingClusters()
	require.NoError(t, err)
	ids := make([]string, 0, len(pending))
	for _, c := range pending {
		ids = append(ids, c.ClusterID)
	}
	return ids
}

func TestIngestDirIngestesUndecidedUnsuppressedClusters(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)

	require.NoError(t, IngestDir(dir, st))

	assert.ElementsMatch(t, []string{"sender-news", "sender-shop", "tag-newsletters"}, pendingIDs(t, st))
}

func TestIngestDirSetsSeenFromGeneratedAt(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)

	require.NoError(t, IngestDir(dir, st))

	pending, err := st.PendingClusters()
	require.NoError(t, err)
	for _, c := range pending {
		assert.Equal(t, "2026-09-02T03:30:00Z", c.FirstSeen)
		assert.Equal(t, "2026-09-02T03:30:00Z", c.LastSeen)
	}
}

func TestIngestDirNeverIngestsTemplateLens(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)

	require.NoError(t, IngestDir(dir, st))

	pending, err := st.PendingClusters()
	require.NoError(t, err)
	for _, c := range pending {
		assert.NotEqual(t, "template-never-ingested", c.ClusterID)
	}
}

func TestIngestDirIsIdempotent(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)

	require.NoError(t, IngestDir(dir, st))
	require.NoError(t, IngestDir(dir, st))

	assert.ElementsMatch(t, []string{"sender-news", "sender-shop", "tag-newsletters"}, pendingIDs(t, st))
}

func TestIngestDirRefreshesRecontainedClustersKeepsAbsentOnes(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)
	require.NoError(t, IngestDir(dir, st))
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportB)

	require.NoError(t, IngestDir(dir, st))

	pending, err := st.PendingClusters()
	require.NoError(t, err)
	byID := make(map[string]Cluster)
	for _, c := range pending {
		byID[c.ClusterID] = c
	}
	require.Contains(t, byID, "sender-news")
	require.Contains(t, byID, "sender-shop")
	require.Contains(t, byID, "tag-newsletters")
	require.Contains(t, byID, "sender-newcomer")

	news := byID["sender-news"]
	assert.Equal(t, 9, news.Count)
	assert.Equal(t, "2026-09-03T03:30:00Z", news.LastSeen)
	assert.Equal(t, "2026-09-02T03:30:00Z", news.FirstSeen)
	assert.Equal(t, []string{"Hello", "Weekly", "Extra"}, news.Examples.SubjectRaw)

	shop := byID["sender-shop"]
	assert.Equal(t, 2, shop.Count)
	assert.Equal(t, "2026-09-02T03:30:00Z", shop.LastSeen)
}

func TestIngestDirSkipsCorruptFilesAndIngestsTheRest(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-broken.json", `{"generated_at": "nope`)
	writeReport(t, dir, "postmanpat-analyze-good.json", reportA)

	require.NoError(t, IngestDir(dir, st))

	assert.ElementsMatch(t, []string{"sender-news", "sender-shop", "tag-newsletters"}, pendingIDs(t, st))
}

func TestIngestDirIgnoresNonReportFiles(t *testing.T) {
	st := openTestStore(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)
	writeReport(t, dir, "notes.txt", "not a report")
	writeReport(t, dir, ".postmanpat-analyze-pending.json.tmp", reportB)

	require.NoError(t, IngestDir(dir, st))

	assert.ElementsMatch(t, []string{"sender-news", "sender-shop", "tag-newsletters"}, pendingIDs(t, st))
}

func TestIngestDirToleratesMissingDirectory(t *testing.T) {
	st := openTestStore(t)

	require.NoError(t, IngestDir(filepath.Join(t.TempDir(), "does-not-exist"), st))

	assert.Empty(t, pendingIDs(t, st))
}
