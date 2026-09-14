package rulesgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecideMarksLaneAndPayloadPersists(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "sender_unsub_lens")}))

	require.NoError(t, st.Decide("c1", LaneWatch, DecisionGenerated, []byte(`[{"name":"a"}]`)))

	pending, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "c1", pending[0].ClusterID)

	all, err := st.AllDecisions()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "c1", all[0].ClusterID)
	assert.Equal(t, LaneWatch, all[0].Lane)
	assert.Equal(t, DecisionGenerated, all[0].Decision)
	assert.Equal(t, `[{"name":"a"}]`, string(all[0].Payload))
}

func TestDecideFullyDecidedLeavesPending(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "sender_unsub_lens")}))

	require.NoError(t, st.Decide("c1", LaneWatch, DecisionDeclined, nil))
	require.NoError(t, st.Decide("c1", LaneOneTimeCleanup, DecisionDeclined, nil))
	require.NoError(t, st.Decide("c1", LaneOngoingCleanup, DecisionDeclined, nil))

	pending, err := st.PendingClusters()
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestDecideUpsertsExistingRow(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "sender_unsub_lens")}))

	require.NoError(t, st.Decide("c1", LaneWatch, DecisionDeclined, nil))
	require.NoError(t, st.Decide("c1", LaneWatch, DecisionSnoozed, nil))

	all, err := st.AllDecisions()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, DecisionSnoozed, all[0].Decision)
}

func TestRevertUndeclinesLaneAndClusterReturnsToPending(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "sender_unsub_lens")}))
	require.NoError(t, st.Decide("c1", LaneWatch, DecisionDeclined, nil))
	require.NoError(t, st.Decide("c1", LaneOneTimeCleanup, DecisionDeclined, nil))
	require.NoError(t, st.Decide("c1", LaneOngoingCleanup, DecisionDeclined, nil))

	require.NoError(t, st.Revert("c1", LaneOngoingCleanup))

	pending, err := st.PendingClusters()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "c1", pending[0].ClusterID)

	all, err := st.AllDecisions()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDecisionsSurviveReopen(t *testing.T) {
	path := t.TempDir() + "/queue.db"
	st, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "list_lens")}))
	require.NoError(t, st.Decide("c1", LaneWatch, DecisionGenerated, []byte(`[{"name":"a"}]`)))
	require.NoError(t, st.Close())

	st2, err := Open(path)
	require.NoError(t, err)
	defer st2.Close()
	all, err := st2.AllDecisions()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, LaneWatch, all[0].Lane)
	assert.Equal(t, DecisionGenerated, all[0].Decision)
}

func TestListDecisionsIncludesOnlyFullyDecidedClusters(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{
		testCluster("fully", "sender_unsub_lens"),
		testCluster("partial", "sender_unsub_lens"),
	}))
	require.NoError(t, st.Decide("fully", LaneWatch, DecisionDeclined, nil))
	require.NoError(t, st.Decide("fully", LaneOneTimeCleanup, DecisionDeclined, nil))
	require.NoError(t, st.Decide("fully", LaneOngoingCleanup, DecisionDeclined, nil))
	require.NoError(t, st.Decide("partial", LaneWatch, DecisionGenerated, nil))

	got, err := st.ListDecisions()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "fully", got[0].Cluster.ClusterID)
	require.Len(t, got[0].Decisions, 3)
}

func TestClearSnoozedRemovesOnlySnoozedRecontained(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("seen", "list_lens"), testCluster("absent", "list_lens")}))
	require.NoError(t, st.Decide("seen", LaneWatch, DecisionSnoozed, nil))
	require.NoError(t, st.Decide("absent", LaneWatch, DecisionSnoozed, nil))
	require.NoError(t, st.Decide("seen", LaneWatch, DecisionDeclined, nil))

	require.NoError(t, st.clearSnoozed([]string{"seen"}, "2030-01-01T00:00:00Z"))

	all, err := st.AllDecisions()
	require.NoError(t, err)
	require.Len(t, all, 2)
	byCluster := map[string]Decision{}
	for _, d := range all {
		byCluster[d.ClusterID] = d.Decision
	}
	assert.Equal(t, DecisionDeclined, byCluster["seen"])
	assert.Equal(t, DecisionSnoozed, byCluster["absent"])
}

func TestClearSnoozedKeepsSnoozeWhenReportNotNewer(t *testing.T) {
	st := openTestStore(t)
	require.NoError(t, st.UpsertClusters([]Cluster{testCluster("c1", "list_lens")}))
	require.NoError(t, st.Decide("c1", LaneWatch, DecisionSnoozed, nil))

	require.NoError(t, st.clearSnoozed([]string{"c1"}, "1999-01-01T00:00:00Z"))

	all, err := st.AllDecisions()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, DecisionSnoozed, all[0].Decision)
}
