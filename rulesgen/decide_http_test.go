package rulesgen

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDecisionServer(t *testing.T) (*Store, http.Handler, string) {
	t.Helper()
	st := openTestStore(t)
	fragDir := t.TempDir()
	return st, NewServer(st, fragDir), fragDir
}

func ingestReportA(t *testing.T, st *Store) {
	t.Helper()
	dir := t.TempDir()
	writeReport(t, dir, "postmanpat-analyze-analyze-inbox.json", reportA)
	require.NoError(t, IngestDir(dir, st))
}

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	return rec
}

func TestDecideGeneratedWritesRuleFragmentAndRedirects(t *testing.T) {
	st, h, fragDir := newDecisionServer(t)
	ingestReportA(t, st)

	rec := postForm(t, h, "/decide", url.Values{
		"cluster_id":   {"sender-shop"},
		"lane":         {"watch"},
		"decision":     {"generated"},
		"name":         {"Shop"},
		"sender_regex": {"shop\\.example\\.org"},
		"action":       {"delete"},
	})
	require.Equal(t, http.StatusSeeOther, rec.Code)

	data, err := os.ReadFile(filepath.Join(fragDir, "watch.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `name: "Shop"`)
	assert.Contains(t, string(data), `shop\.example\.org`)

	decided, err := st.AllDecisions()
	require.NoError(t, err)
	require.Len(t, decided, 1)
	assert.Equal(t, DecisionGenerated, decided[0].Decision)
}

func TestDecideIgnoredWritesIgnoreFragmentAndCrossOfferCoversCleanupLanes(t *testing.T) {
	st, h, fragDir := newDecisionServer(t)
	ingestReportA(t, st)

	rec := postForm(t, h, "/decide", url.Values{
		"cluster_id":          {"sender-news"},
		"lane":                {"watch"},
		"decision":            {"ignored"},
		"also_ignore_cleanup": {"true"},
	})
	require.Equal(t, http.StatusSeeOther, rec.Code)

	data, err := os.ReadFile(filepath.Join(fragDir, "ignore.yaml"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "sender_domains")
	assert.Contains(t, body, "watch:")
	assert.Contains(t, body, "cleanup:")

	all, err := st.AllDecisions()
	require.NoError(t, err)
	lanes := map[Lane]Decision{}
	for _, d := range all {
		lanes[d.Lane] = d.Decision
	}
	assert.Equal(t, DecisionIgnored, lanes[LaneWatch])
	assert.Equal(t, DecisionIgnored, lanes[LaneOneTimeCleanup])
	assert.Equal(t, DecisionIgnored, lanes[LaneOngoingCleanup])

	pending := pendingIDs(t, st)
	assert.NotContains(t, pending, "sender-news")
}

func TestDecideSnoozedHidesLane(t *testing.T) {
	st, h, _ := newDecisionServer(t)
	ingestReportA(t, st)

	rec := postForm(t, h, "/decide", url.Values{
		"cluster_id": {"tag-newsletters"},
		"lane":       {"watch"},
		"decision":   {"snoozed"},
	})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.NotContains(t, pendingIDs(t, st), "tag-newsletters")
}

func TestDecideRejectsUnknownLane(t *testing.T) {
	st, h, _ := newDecisionServer(t)
	ingestReportA(t, st)

	rec := postForm(t, h, "/decide", url.Values{
		"cluster_id": {"sender-news"},
		"lane":       {"nonsense_lane"},
		"decision":   {"declined"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDecideRejectsUnknownDecision(t *testing.T) {
	st, h, _ := newDecisionServer(t)
	ingestReportA(t, st)

	rec := postForm(t, h, "/decide", url.Values{
		"cluster_id": {"sender-news"},
		"lane":       {"watch"},
		"decision":   {"explode"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRevertReturnsClusterToQueueAndRerenders(t *testing.T) {
	st, h, fragDir := newDecisionServer(t)
	ingestReportA(t, st)
	require.NoError(t, st.Decide("sender-news", LaneWatch, DecisionDeclined, nil))
	require.NoError(t, st.Decide("sender-news", LaneOneTimeCleanup, DecisionDeclined, nil))
	require.NoError(t, st.Decide("sender-news", LaneOngoingCleanup, DecisionDeclined, nil))

	code, body := get(t, h, "/decisions")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "sender-news")

	rec := postForm(t, h, "/revert", url.Values{
		"cluster_id": {"sender-news"},
		"lane":       {"watch"},
	})
	require.Equal(t, http.StatusSeeOther, rec.Code)

	assert.Contains(t, pendingIDs(t, st), "sender-news")

	data, _ := os.ReadFile(filepath.Join(fragDir, "watch.yaml"))
	assert.Equal(t, "rules: []\n", string(data))
}

func TestQueuePageOffersCleanupFoldersAndAgeWindow(t *testing.T) {
	st, h, _ := newDecisionServer(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "sender_unsub_lens")}))

	_, body := get(t, h, "/")

	assert.Contains(t, body, `name="folders" value="INBOX"`)
	assert.Contains(t, body, `name="age_window_min" value="30d"`)
	assert.Contains(t, body, `name="age_window_min" value=""`)
}

func TestQueuePageSuppressedLanesOfferNoControls(t *testing.T) {
	st, h, _ := newDecisionServer(t)
	require.NoError(t, st.UpsertClusters([]Cluster{{
		ClusterID:  "c2",
		Lens:       "sender_unsub_lens",
		Keys:       map[string]any{"SenderDomains": []any{"shop.example.org"}},
		Count:      1,
		Suppressed: []string{"cleanup"},
		LastSeen:   "2026-09-02T03:30:00Z",
	}}))

	_, body := get(t, h, "/")

	assert.Contains(t, body, "suppressed: cleanup")
	assert.NotContains(t, body, "ongoing_cleanup")
	assert.NotContains(t, body, "one_time_cleanup")

	rec := postForm(t, h, "/decide", url.Values{
		"cluster_id": {"c2"},
		"lane":       {"ongoing_cleanup"},
		"decision":   {"declined"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDecisionsPageShowsDecidedClusterAndReversion(t *testing.T) {
	st, h, _ := newDecisionServer(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "sender_unsub_lens")}))
	require.NoError(t, st.Decide("c1", LaneWatch, DecisionGenerated, nil))
	require.NoError(t, st.Decide("c1", LaneOneTimeCleanup, DecisionDeclined, nil))
	require.NoError(t, st.Decide("c1", LaneOngoingCleanup, DecisionSnoozed, nil))

	code, body := get(t, h, "/decisions")

	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "c1")
	assert.Contains(t, body, "generated")
	assert.Contains(t, body, "declined")
	assert.Contains(t, body, "snoozed")
	assert.Contains(t, body, "/revert")
}
