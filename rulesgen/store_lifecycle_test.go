package rulesgen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	// Test that store can be created and used
	store, err := NewStore(dbPath)
	require.NoError(t, err, "should create store")
	
	// Store a decision
	d := Decision{
		Lens:      "list_lens",
		ClusterID: "test123",
		Type:      "watch",
		Action:    "delete",
	}
	
	id, err := store.CreateDecision(d)
	require.NoError(t, err, "should store decision")
	assert.Greater(t, id, int64(0))
	
	// Retrieve it back
	decisions, err := store.GetDecisions("list_lens", "test123")
	require.NoError(t, err, "should retrieve decision")
	require.Len(t, decisions, 1)
	assert.Equal(t, id, decisions[0].ID)
	
	// Close store
	err = store.Close()
	require.NoError(t, err, "should close store")
	
	// Try to use closed store (should fail)
	_, err = store.GetDecisions("list_lens", "test123")
	require.Error(t, err, "should error on closed store")
	assert.Contains(t, err.Error(), "closed")
}

func TestLoadServerStore(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := LoadServerStore("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "store path is empty")
	})
	
	t.Run("creates directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "subdir", "test.db")
		
		store, err := LoadServerStore(dbPath)
		require.NoError(t, err)
		require.NotNil(t, store)
		defer store.Close()
		
		// Should be able to use the store
		d := Decision{
			Lens:      "test",
			ClusterID: "test",
			Type:      "ignore",
		}
		
		_, err = store.CreateDecision(d)
		require.NoError(t, err)
	})
	
	t.Run("existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")
		
		store, err := LoadServerStore(dbPath)
		require.NoError(t, err)
		require.NotNil(t, store)
		defer store.Close()
		
		// Should be able to use the store
		d := Decision{
			Lens:      "test",
			ClusterID: "test",
			Type:      "ignore",
		}
		
		_, err = store.CreateDecision(d)
		require.NoError(t, err)
	})
}

func TestServerStoreNotClosed(t *testing.T) {
	// This test verifies that NewServer doesn't immediately close the store
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	// Create a store first
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	
	// Store a decision
	d := Decision{
		Lens:      "list_lens",
		ClusterID: "test123",
		Type:      "watch",
		Action:    "delete",
	}
	
	_, err = store.CreateDecision(d)
	require.NoError(t, err)
	
	// Create server config using NewServerConfig
	config := NewServerConfig(
		WithRulesGenStore(dbPath),
		WithPort(8080),
		WithRulesConfig("/tmp/test.yaml"),
		WithAddr("imap.example.com:993"),
		WithCreds("test", "test"),
		WithWatchOut("/tmp/watch.yml"),
		WithCleanupOut("/tmp/cleanup.yml"),
		WithOneTimeOut("/tmp/onetime.yml"),
	)
	
	// Create server - this should NOT close the store
	server, err := NewServer(config)
	require.NoError(t, err)
	require.NotNil(t, server)
	
	// Store should still be usable through server
	serverStore := server.GetStore()
	require.NotNil(t, serverStore)
	
	// Should be able to retrieve the decision we stored earlier
	decisions, err := serverStore.GetDecisions("list_lens", "test123")
	require.NoError(t, err, "store should still be open and usable")
	require.Len(t, decisions, 1)
	
	// Clean up
	err = server.Close()
	require.NoError(t, err)
}