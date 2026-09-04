package rulesgen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testCluster(id, lens string) Cluster {
	return Cluster{
		ClusterID:  id,
		Lens:       lens,
		Keys:       map[string]any{"SenderDomains": []any{"news.example.com"}},
		Count:      3,
		LatestDate: "2026-09-01T10:00:00Z",
		Examples:   Examples{SubjectRaw: []string{"Hello"}, Recipients: []string{"aaron@example.com"}},
		Signals:    Signals{HasListUnsubscribe: true, PrecedenceCategories: map[string]int{"list": 1}},
		LastSeen:   "2026-09-02T03:30:00Z",
	}
}

func TestUpsertInsertsNewClusters(t *testing.T) {
	st := openTestStore(t)
	c := testCluster("c1", "sender_unsub_lens")

	require.NoError(t, st.UpsertClusters([]Cluster{c}))

	got, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, got, 1)
	want := c
	want.FirstSeen = c.LastSeen
	assert.Equal(t, want, got[0])
}

func TestUpsertIsIdempotent(t *testing.T) {
	st := openTestStore(t)
	c := testCluster("c1", "sender_unsub_lens")

	require.NoError(t, st.UpsertClusters([]Cluster{c}))
	require.NoError(t, st.UpsertClusters([]Cluster{c}))

	got, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestUpsertRefreshesMutableFieldsKeepsFirstSeen(t *testing.T) {
	st := openTestStore(t)
	c := testCluster("c1", "sender_unsub_lens")
	require.NoError(t, st.UpsertClusters([]Cluster{c}))

	fresh := c
	fresh.Count = 9
	fresh.LastSeen = "2026-09-03T03:30:00Z"
	fresh.Examples = Examples{SubjectRaw: []string{"Hello", "Again"}}
	require.NoError(t, st.UpsertClusters([]Cluster{fresh}))

	got, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 9, got[0].Count)
	assert.Equal(t, "2026-09-03T03:30:00Z", got[0].LastSeen)
	assert.Equal(t, "2026-09-02T03:30:00Z", got[0].FirstSeen)
	assert.Equal(t, Examples{SubjectRaw: []string{"Hello", "Again"}}, got[0].Examples)
}

func TestPendingExcludesSuppressedForBoth(t *testing.T) {
	st := openTestStore(t)
	both := testCluster("c1", "sender_unsub_lens")
	both.Suppressed = []string{"watch", "cleanup"}
	half := testCluster("c2", "sender_unsub_lens")
	half.Suppressed = []string{"cleanup"}

	require.NoError(t, st.UpsertClusters([]Cluster{both, half}))

	got, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "c2", got[0].ClusterID)
}

func TestPendingRespectsLaneDecisions(t *testing.T) {
	st := openTestStore(t)
	allDecided := testCluster("c1", "sender_unsub_lens")
	oneLaneDecided := testCluster("c2", "sender_unsub_lens")
	tagWatchDecided := testCluster("c3", "recipient_tag_lens")
	require.NoError(t, st.UpsertClusters([]Cluster{allDecided, oneLaneDecided, tagWatchDecided}))

	_, err := st.db.Exec(`INSERT INTO decisions (cluster_id, lane, decision, decided_at) VALUES (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?)`,
		"c1", "watch", "declined", "2026-09-03T10:00:00Z",
		"c1", "one_time_cleanup", "declined", "2026-09-03T10:00:00Z",
		"c1", "ongoing_cleanup", "declined", "2026-09-03T10:00:00Z",
		"c2", "watch", "generated", "2026-09-03T10:00:00Z",
	)
	require.NoError(t, err)
	_, err = st.db.Exec(`INSERT INTO decisions (cluster_id, lane, decision, decided_at) VALUES (?, ?, ?, ?)`,
		"c3", "watch", "ignored", "2026-09-03T10:00:00Z",
	)
	require.NoError(t, err)

	got, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "c2", got[0].ClusterID)
}

func TestPendingOrdersByLastSeenDesc(t *testing.T) {
	st := openTestStore(t)
	old := testCluster("c1", "sender_unsub_lens")
	recent := testCluster("c2", "sender_unsub_lens")
	recent.LastSeen = "2026-09-05T03:30:00Z"
	mid := testCluster("c3", "sender_unsub_lens")
	mid.LastSeen = "2026-09-04T03:30:00Z"

	require.NoError(t, st.UpsertClusters([]Cluster{old, recent, mid}))

	got, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"c2", "c3", "c1"}, []string{got[0].ClusterID, got[1].ClusterID, got[2].ClusterID})
}
