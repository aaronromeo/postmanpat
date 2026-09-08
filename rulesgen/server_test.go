package rulesgen

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*Store, http.Handler) {
	t.Helper()
	st := openTestStore(t)
	return st, NewServer(st)
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestQueuePageShowsPendingClusters(t *testing.T) {
	st, h := newTestServer(t)
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)
	require.NoError(t, IngestDir(dir, st))

	code, body := get(t, h, "/")

	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "sender-news")
	assert.Contains(t, body, "sender_unsub_lens")
	assert.Contains(t, body, "news.example.com")
	assert.Contains(t, body, "Weekly")
	assert.Contains(t, body, "2026-09-02T03:30:00Z")
	assert.Contains(t, body, "sender-shop")
	assert.Contains(t, body, "cleanup")
	assert.Contains(t, body, "tag-newsletters")
	assert.NotContains(t, body, "template-never-ingested")
	assert.NotContains(t, body, "list-both-suppressed")
}

func TestQueuePageEscapesClusterData(t *testing.T) {
	st, h := newTestServer(t)
	require.NoError(t, st.UpsertClusters([]Cluster{{
		ClusterID: "xss",
		Lens:      "sender_unsub_lens",
		Keys:      map[string]any{"SenderDomains": []any{"evil.example.com"}},
		Count:     1,
		Examples:  Examples{SubjectRaw: []string{"<script>alert(1)</script>"}},
		LastSeen:  "2026-09-02T03:30:00Z",
	}}))

	_, body := get(t, h, "/")

	assert.NotContains(t, body, "<script>")
	assert.Contains(t, body, "&lt;script&gt;")
}

func TestQueuePageEmptyStore(t *testing.T) {
	_, h := newTestServer(t)

	code, body := get(t, h, "/")

	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, strings.ToLower(body), "no pending")
}

func TestHealthzReturnsOK(t *testing.T) {
	_, h := newTestServer(t)

	code, body := get(t, h, "/healthz")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok\n", body)
}
