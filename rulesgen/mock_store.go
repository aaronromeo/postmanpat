package rulesgen

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MockStore is a test double for Store that stores decisions in memory
type MockStore struct {
	decisions       map[string]Decision
	decidedClusters map[string]bool
	closed          bool
}

// NewMockStore creates a new mock store for testing
func NewMockStore() *MockStore {
	return &MockStore{
		decisions:       make(map[string]Decision),
		decidedClusters: make(map[string]bool),
	}
}

// CreateDecision stores a decision in memory
func (m *MockStore) CreateDecision(d Decision) (int64, error) {
	if m.closed {
		return 0, fmt.Errorf("store is closed")
	}
	key := fmt.Sprintf("%s:%s:%s", d.Lens, d.ClusterID, d.Type)
	d.ID = int64(len(m.decisions) + 1)
	d.CreatedAt = time.Now().UTC()
	m.decisions[key] = d
	m.decidedClusters[fmt.Sprintf("%s:%s", d.Lens, d.ClusterID)] = true
	return d.ID, nil
}

// GetDecisions returns decisions for a specific cluster
func (m *MockStore) GetDecisions(lens string, clusterID string) ([]Decision, error) {
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	var result []Decision
	for key, d := range m.decisions {
		if d.Lens == lens && d.ClusterID == clusterID {
			result = append(result, d)
			_ = key // silence unused warning
		}
	}
	return result, nil
}

// HasDecision checks if any decision exists for a cluster
func (m *MockStore) HasDecision(lens string, clusterID string) (bool, error) {
	if m.closed {
		return false, fmt.Errorf("store is closed")
	}
	return m.decidedClusters[fmt.Sprintf("%s:%s", lens, clusterID)], nil
}

// GetAllDecisions returns all stored decisions
func (m *MockStore) GetAllDecisions() ([]Decision, error) {
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	var result []Decision
	for _, d := range m.decisions {
		result = append(result, d)
	}
	return result, nil
}

// GetAllDecidedClusterIDs returns a map of decided cluster IDs
func (m *MockStore) GetAllDecidedClusterIDs() (map[string]bool, error) {
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	// Return a copy to prevent external modification
	result := make(map[string]bool)
	for k, v := range m.decidedClusters {
		result[k] = v
	}
	return result, nil
}

func (m *MockStore) DeleteDecision(lens string, clusterID string, decisionType string) error {
	if m.closed {
		return fmt.Errorf("store is closed")
	}
	key := fmt.Sprintf("%s:%s:%s", lens, clusterID, decisionType)
	if _, exists := m.decisions[key]; exists {
		delete(m.decisions, key)
		// Check if any decisions remain for this lens:clusterID
		hasOther := false
		for k := range m.decisions {
			if strings.HasPrefix(k, fmt.Sprintf("%s:%s:", lens, clusterID)) {
				hasOther = true
				break
			}
		}
		if !hasOther {
			delete(m.decidedClusters, fmt.Sprintf("%s:%s", lens, clusterID))
		}
	}
	return nil
}

func (m *MockStore) GetDecisionsByType(decisionType string) ([]Decision, error) {
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	var result []Decision
	for _, d := range m.decisions {
		if d.Type == decisionType {
			result = append(result, d)
		}
	}
	// Sort by CreatedAt descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

// Close marks the store as closed
func (m *MockStore) Close() error {
	m.closed = true
	return nil
}

// BeginTransaction returns a mock transaction (no-op for testing)
func (m *MockStore) BeginTransaction() (*sql.Tx, error) {
	if m.closed {
		return nil, fmt.Errorf("store is closed")
	}
	// Return nil - tests can check if store methods are called
	return nil, nil
}

// CreateDecisionTx creates a decision within a transaction
func (m *MockStore) CreateDecisionTx(tx *sql.Tx, d Decision) (int64, error) {
	if m.closed {
		return 0, fmt.Errorf("store is closed")
	}
	// Ignore tx for mock - just call regular CreateDecision
	return m.CreateDecision(d)
}

// Verify MockStore implements StoreInterface
var _ Store = (*MockStore)(nil)
