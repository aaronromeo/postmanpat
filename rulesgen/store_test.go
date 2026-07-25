package rulesgen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStore(t *testing.T) (*SqlStore, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { store.Close() })
	return store, dbPath
}

func TestStore_CreateDecision(t *testing.T) {
	t.Run("basic insert", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		id, err := store.CreateDecision(d)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
		assert.Equal(t, id, decisions[0].ID)
		assert.Equal(t, "list_lens", decisions[0].Lens)
		assert.Equal(t, "abc123", decisions[0].ClusterID)
		assert.Equal(t, "watch", decisions[0].Type)
		assert.Equal(t, "delete", decisions[0].Action)
		assert.Equal(t, "", decisions[0].Destination)
		assert.Equal(t, "", decisions[0].AgeWindow)
		assert.False(t, decisions[0].CreatedAt.IsZero())
	})

	t.Run("insert with all fields", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:        "sender_unsub_lens",
			ClusterID:   "def456",
			Type:        "cleanup",
			Action:      "move",
			Destination: "Archive",
			AgeWindow:   "30d",
		}

		id, err := store.CreateDecision(d)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		decisions, err := store.GetDecisions("sender_unsub_lens", "def456")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
		assert.Equal(t, "cleanup", decisions[0].Type)
		assert.Equal(t, "move", decisions[0].Action)
		assert.Equal(t, "Archive", decisions[0].Destination)
		assert.Equal(t, "30d", decisions[0].AgeWindow)
	})

	t.Run("duplicate prevention via ON CONFLICT update", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d1 := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		id1, err := store.CreateDecision(d1)
		require.NoError(t, err)

		d2 := Decision{
			Lens:        "list_lens",
			ClusterID:   "abc123",
			Type:        "watch",
			Action:      "move",
			Destination: "Trash",
		}

		id2, err := store.CreateDecision(d2)
		require.NoError(t, err)
		assert.Equal(t, id1, id2, "should return same ID for update")

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
		assert.Equal(t, "move", decisions[0].Action)
		assert.Equal(t, "Trash", decisions[0].Destination)
	})

	t.Run("different types for same cluster", func(t *testing.T) {
		store, _ := setupTestStore(t)
		watch := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		cleanup := Decision{
			Lens:        "list_lens",
			ClusterID:   "abc123",
			Type:        "cleanup",
			Action:      "move",
			Destination: "Archive",
			AgeWindow:   "7d",
		}

		wid, err := store.CreateDecision(watch)
		require.NoError(t, err)
		cid, err := store.CreateDecision(cleanup)
		require.NoError(t, err)
		assert.NotEqual(t, wid, cid)

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		assert.Len(t, decisions, 2)

		typeCounts := map[string]int{}
		for _, d := range decisions {
			typeCounts[d.Type]++
		}
		assert.Equal(t, 1, typeCounts["watch"])
		assert.Equal(t, 1, typeCounts["cleanup"])
	})
}

func TestStore_GetDecisions(t *testing.T) {
	t.Run("empty results", func(t *testing.T) {
		store, _ := setupTestStore(t)
		decisions, err := store.GetDecisions("nonexistent", "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, decisions)
	})

	t.Run("ordering by created_at", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d1 := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}
		d2 := Decision{
			Lens:        "list_lens",
			ClusterID:   "abc123",
			Type:        "cleanup",
			Action:      "move",
			Destination: "Archive",
			AgeWindow:   "30d",
		}

		_, err := store.CreateDecision(d1)
		require.NoError(t, err)
		_, err = store.CreateDecision(d2)
		require.NoError(t, err)

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 2)
		// Both should exist, order not guaranteed with same second timestamp
		typeCounts := map[string]int{}
		for _, d := range decisions {
			typeCounts[d.Type]++
		}
		assert.Equal(t, 1, typeCounts["watch"])
		assert.Equal(t, 1, typeCounts["cleanup"])
	})

	t.Run("multiple clusters", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d1 := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}
		d2 := Decision{
			Lens:        "sender_unsub_lens",
			ClusterID:   "def456",
			Type:        "cleanup",
			Action:      "move",
			Destination: "Archive",
			AgeWindow:   "30d",
		}

		_, err := store.CreateDecision(d1)
		require.NoError(t, err)
		_, err = store.CreateDecision(d2)
		require.NoError(t, err)

		decisions1, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions1, 1)
		assert.Equal(t, "watch", decisions1[0].Type)

		decisions2, err := store.GetDecisions("sender_unsub_lens", "def456")
		require.NoError(t, err)
		require.Len(t, decisions2, 1)
		assert.Equal(t, "cleanup", decisions2[0].Type)
	})
}

func TestStore_HasDecision(t *testing.T) {
	t.Run("true/false cases", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		_, err := store.CreateDecision(d)
		require.NoError(t, err)

		has, err := store.HasDecision("list_lens", "abc123")
		require.NoError(t, err)
		assert.True(t, has)

		has, err = store.HasDecision("list_lens", "nonexistent")
		require.NoError(t, err)
		assert.False(t, has)

		has, err = store.HasDecision("nonexistent", "abc123")
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("non-existent clusters", func(t *testing.T) {
		store, _ := setupTestStore(t)
		has, err := store.HasDecision("nonexistent", "nonexistent")
		require.NoError(t, err)
		assert.False(t, has)
	})
}

func TestStore_GetAllDecisions(t *testing.T) {
	t.Run("multiple entries", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d1 := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}
		d2 := Decision{
			Lens:        "sender_unsub_lens",
			ClusterID:   "def456",
			Type:        "cleanup",
			Action:      "move",
			Destination: "Archive",
			AgeWindow:   "30d",
		}

		_, err := store.CreateDecision(d1)
		require.NoError(t, err)
		_, err = store.CreateDecision(d2)
		require.NoError(t, err)

		decisions, err := store.GetAllDecisions()
		require.NoError(t, err)
		require.Len(t, decisions, 2)
		// Both should exist
		typeCounts := map[string]int{}
		for _, d := range decisions {
			typeCounts[d.Type]++
		}
		assert.Equal(t, 1, typeCounts["watch"])
		assert.Equal(t, 1, typeCounts["cleanup"])
	})

	t.Run("multiple decisions exist", func(t *testing.T) {
		store, _ := setupTestStore(t)
		decisions := []Decision{
			{Lens: "list_lens", ClusterID: "1", Type: "watch", Action: "delete"},
			{Lens: "list_lens", ClusterID: "2", Type: "cleanup", Action: "move", Destination: "Archive", AgeWindow: "30d"},
			{Lens: "sender_unsub_lens", ClusterID: "3", Type: "ignore"},
		}

		for _, d := range decisions {
			_, err := store.CreateDecision(d)
			require.NoError(t, err)
		}

		all, err := store.GetAllDecisions()
		require.NoError(t, err)
		require.Len(t, all, 3)
		
		typeCounts := map[string]int{}
		for _, d := range all {
			typeCounts[d.Type]++
		}
		assert.Equal(t, 1, typeCounts["watch"])
		assert.Equal(t, 1, typeCounts["cleanup"])
		assert.Equal(t, 1, typeCounts["ignore"])
	})
}

func TestStore_GetAllDecidedClusterIDs(t *testing.T) {
	t.Run("map construction", func(t *testing.T) {
		store, _ := setupTestStore(t)
		decisions := []Decision{
			{Lens: "list_lens", ClusterID: "abc123", Type: "watch", Action: "delete"},
			{Lens: "list_lens", ClusterID: "abc123", Type: "cleanup", Action: "move", Destination: "Archive", AgeWindow: "30d"},
			{Lens: "sender_unsub_lens", ClusterID: "def456", Type: "watch", Action: "delete"},
			{Lens: "recipient_tag_lens", ClusterID: "ghi789", Type: "ignore"},
		}

		for _, d := range decisions {
			_, err := store.CreateDecision(d)
			require.NoError(t, err)
		}

		decided, err := store.GetAllDecidedClusterIDs()
		require.NoError(t, err)
		assert.Len(t, decided, 3, "should have 3 unique lens:cluster_id combos")

		assert.True(t, decided["list_lens:abc123"])
		assert.True(t, decided["sender_unsub_lens:def456"])
		assert.True(t, decided["recipient_tag_lens:ghi789"])
		assert.False(t, decided["nonexistent:foo"])

		t.Run("key formatting", func(t *testing.T) {
			for key := range decided {
				assert.Contains(t, key, ":", "key should contain colon")
				parts := map[string]string{
					"list_lens:abc123":           "list_lens",
					"sender_unsub_lens:def456":   "sender_unsub_lens",
					"recipient_tag_lens:ghi789": "recipient_tag_lens",
				}
				lens, ok := parts[key]
				require.True(t, ok, "unexpected key: %s", key)
				assert.Contains(t, key, lens+":", "key should start with lens")
			}
		})
	})

	t.Run("empty database", func(t *testing.T) {
		store, _ := setupTestStore(t)
		decided, err := store.GetAllDecidedClusterIDs()
		require.NoError(t, err)
		assert.Empty(t, decided)
	})
}

func TestStore_DeleteDecision(t *testing.T) {
	t.Run("removal", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		_, err := store.CreateDecision(d)
		require.NoError(t, err)

		has, err := store.HasDecision("list_lens", "abc123")
		require.NoError(t, err)
		assert.True(t, has)

		err = store.DeleteDecision("list_lens", "abc123", "watch")
		require.NoError(t, err)

		has, err = store.HasDecision("list_lens", "abc123")
		require.NoError(t, err)
		assert.False(t, has)

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		assert.Empty(t, decisions)
	})

	t.Run("idempotent delete", func(t *testing.T) {
		store, _ := setupTestStore(t)
		err := store.DeleteDecision("nonexistent", "nonexistent", "watch")
		require.NoError(t, err, "deleting non-existent decision should not error")
	})

	t.Run("specific type deletion", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d1 := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}
		d2 := Decision{
			Lens:        "list_lens",
			ClusterID:   "abc123",
			Type:        "cleanup",
			Action:      "move",
			Destination: "Archive",
			AgeWindow:   "30d",
		}

		_, err := store.CreateDecision(d1)
		require.NoError(t, err)
		_, err = store.CreateDecision(d2)
		require.NoError(t, err)

		has, err := store.HasDecision("list_lens", "abc123")
		require.NoError(t, err)
		assert.True(t, has)

		err = store.DeleteDecision("list_lens", "abc123", "watch")
		require.NoError(t, err)

		has, err = store.HasDecision("list_lens", "abc123")
		require.NoError(t, err)
		assert.True(t, has, "should still have cleanup decision")

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
		assert.Equal(t, "cleanup", decisions[0].Type)
	})
}

func TestStore_CreateDecisionTx(t *testing.T) {
	t.Run("transaction success", func(t *testing.T) {
		store, _ := setupTestStore(t)
		tx, err := store.BeginTransaction()
		require.NoError(t, err)
		defer tx.Rollback()

		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		id, err := store.CreateDecisionTx(tx, d)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		require.NoError(t, tx.Commit())

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
		assert.Equal(t, id, decisions[0].ID)
	})

	t.Run("transaction rollback", func(t *testing.T) {
		store, _ := setupTestStore(t)
		tx, err := store.BeginTransaction()
		require.NoError(t, err)

		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		id, err := store.CreateDecisionTx(tx, d)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		require.NoError(t, tx.Rollback())

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		assert.Empty(t, decisions, "should be empty after rollback")
	})

	t.Run("exec outside transaction", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		id, err := store.CreateDecision(d)
		require.NoError(t, err)

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
		assert.Equal(t, id, decisions[0].ID)
	})
}

func TestStore_SchemaInitialization(t *testing.T) {
	t.Run("schema verification", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		
		store, err := NewStore(dbPath)
		require.NoError(t, err, "failed to create store")
		defer store.Close()

		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "delete",
		}

		id, err := store.CreateDecision(d)
		require.NoError(t, err)
		assert.Greater(t, id, int64(0))

		decisions, err := store.GetDecisions("list_lens", "abc123")
		require.NoError(t, err)
		require.Len(t, decisions, 1)
	})

	t.Run("database file creation", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		
		store, err := NewStore(dbPath)
		require.NoError(t, err, "should create store with new file")
		defer store.Close()

		d := Decision{
			Lens:      "list_lens",
			ClusterID: "test",
			Type:      "watch",
			Action:    "delete",
		}

		_, err = store.CreateDecision(d)
		require.NoError(t, err)
	})

	t.Run("invalid decision type", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "invalid",
			Action:    "delete",
		}

		_, err := store.CreateDecision(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CHECK constraint failed", "should violate type constraint")
	})

	t.Run("invalid action", func(t *testing.T) {
		store, _ := setupTestStore(t)
		d := Decision{
			Lens:      "list_lens",
			ClusterID: "abc123",
			Type:      "watch",
			Action:    "invalid",
		}

		_, err := store.CreateDecision(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CHECK constraint failed", "should violate action constraint")
	})
}