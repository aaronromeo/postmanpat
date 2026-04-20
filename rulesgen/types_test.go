package rulesgen

import (
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecision_Validate(t *testing.T) {
	t.Run("valid ignore decision", func(t *testing.T) {
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "ignore",
			CreatedAt: time.Now().UTC(),
		}
		assert.NoError(t, d.Validate())
	})

	t.Run("valid watch decision", func(t *testing.T) {
		d := Decision{
			Lens:      "sender_unsub_lens",
			ClusterID: "def456",
			Type:      "watch",
			Action:    "move",
			Destination: "Archive",
			CreatedAt: time.Now().UTC(),
		}
		assert.NoError(t, d.Validate())
	})

	t.Run("valid cleanup decision", func(t *testing.T) {
		d := Decision{
			Lens:        "recipient_tag_lens",
			ClusterID:   "ghi789",
			Type:        "cleanup",
			Action:      "delete",
			AgeWindow:   "30d",
			CreatedAt:   time.Now().UTC(),
		}
		assert.NoError(t, d.Validate())
	})

	t.Run("missing lens", func(t *testing.T) {
		d := Decision{
			ClusterID: "abc123",
			Type:      "ignore",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lens is required")
	})

	t.Run("missing cluster_id", func(t *testing.T) {
		d := Decision{
			Lens: "list_lens",
			Type: "ignore",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cluster_id is required")
	})

	t.Run("invalid type", func(t *testing.T) {
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "invalid",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type must be ignore, watch, or cleanup")
	})

	t.Run("watch missing action", func(t *testing.T) {
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action must be delete or move")
	})

	t.Run("cleanup missing action", func(t *testing.T) {
		d := Decision{
			Lens:        "list_lens",
			ClusterID:   "abc123",
			Type:        "cleanup",
			AgeWindow:   "30d",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action must be delete or move")
	})

	t.Run("move missing destination", func(t *testing.T) {
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "move",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destination is required for move action")
	})

	t.Run("cleanup missing age_window", func(t *testing.T) {
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "cleanup",
			Action:    "delete",
		}
		err := d.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "age_window is required for cleanup")
	})
}

func TestNewDecisionFromCluster(t *testing.T) {
	createTestCluster := func() analysis.Cluster {
		return analysis.Cluster{
			ClusterID:  "list_lens:test123",
			Count:      10,
			LatestDate: time.Now().UTC(),
			Keys: map[string]any{
				"ListID": "example-list-id",
			},
			Signals: analysis.ClusterSignals{
				HasListID:          true,
				HasListUnsubscribe: true,
			},
			Examples: analysis.ClusterExamples{
				SubjectRaw: []string{"Test Subject"},
				Recipients: []string{"user@example.com"},
			},
		}
	}

	t.Run("valid ignore decision", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "ignore", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, "list_lens", d.Lens)
		assert.Equal(t, "test123", d.ClusterID)
		assert.Equal(t, "ignore", d.Type)
		assert.Equal(t, "", d.Action)
		assert.Equal(t, "", d.Destination)
		assert.Equal(t, "", d.AgeWindow)
		assert.False(t, d.CreatedAt.IsZero())
	})

	t.Run("valid watch delete decision", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "watch", "delete", "", "")
		require.NoError(t, err)
		assert.Equal(t, "list_lens", d.Lens)
		assert.Equal(t, "test123", d.ClusterID)
		assert.Equal(t, "watch", d.Type)
		assert.Equal(t, "delete", d.Action)
		assert.Equal(t, "", d.Destination)
		assert.Equal(t, "", d.AgeWindow)
	})

	t.Run("valid watch move decision", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "watch", "move", "Archive", "")
		require.NoError(t, err)
		assert.Equal(t, "list_lens", d.Lens)
		assert.Equal(t, "test123", d.ClusterID)
		assert.Equal(t, "watch", d.Type)
		assert.Equal(t, "move", d.Action)
		assert.Equal(t, "Archive", d.Destination)
		assert.Equal(t, "", d.AgeWindow)
	})

	t.Run("valid cleanup delete decision", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "cleanup", "delete", "", "30d")
		require.NoError(t, err)
		assert.Equal(t, "list_lens", d.Lens)
		assert.Equal(t, "test123", d.ClusterID)
		assert.Equal(t, "cleanup", d.Type)
		assert.Equal(t, "delete", d.Action)
		assert.Equal(t, "", d.Destination)
		assert.Equal(t, "30d", d.AgeWindow)
	})

	t.Run("valid cleanup move decision", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "cleanup", "move", "Trash", "7d")
		require.NoError(t, err)
		assert.Equal(t, "list_lens", d.Lens)
		assert.Equal(t, "test123", d.ClusterID)
		assert.Equal(t, "cleanup", d.Type)
		assert.Equal(t, "move", d.Action)
		assert.Equal(t, "Trash", d.Destination)
		assert.Equal(t, "7d", d.AgeWindow)
	})

	t.Run("invalid cluster ID format", func(t *testing.T) {
		cluster := createTestCluster()
		cluster.ClusterID = "no-colon"
		d, err := NewDecisionFromCluster(cluster, "ignore", "", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cluster ID format")
		assert.Empty(t, d.Lens)
	})

	t.Run("invalid decision type", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "invalid", "", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type must be ignore, watch, or cleanup")
		assert.Empty(t, d.Lens)
	})

	t.Run("watch missing action", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "watch", "", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "action must be delete or move")
		assert.Empty(t, d.Lens)
	})

	t.Run("cleanup missing age_window", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "cleanup", "delete", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "age_window is required for cleanup")
		assert.Empty(t, d.Lens)
	})

	t.Run("move missing destination", func(t *testing.T) {
		cluster := createTestCluster()
		d, err := NewDecisionFromCluster(cluster, "watch", "move", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destination is required for move action")
		assert.Empty(t, d.Lens)
	})

	t.Run("different lens types", func(t *testing.T) {
		tests := []struct {
			clusterID string
			expectedLens string
			expectedClusterID string
		}{
			{"list_lens:abc123", "list_lens", "abc123"},
			{"sender_unsub_lens:def456", "sender_unsub_lens", "def456"},
			{"recipient_tag_lens:ghi789", "recipient_tag_lens", "ghi789"},
			{"list_lens:complex:id:with:colons", "list_lens", "complex:id:with:colons"},
		}

		for _, tt := range tests {
			t.Run(tt.clusterID, func(t *testing.T) {
				cluster := createTestCluster()
				cluster.ClusterID = tt.clusterID
				d, err := NewDecisionFromCluster(cluster, "ignore", "", "", "")
				require.NoError(t, err)
				assert.Equal(t, tt.expectedLens, d.Lens)
				assert.Equal(t, tt.expectedClusterID, d.ClusterID)
			})
		}
	})
}