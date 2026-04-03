package rulesgen

import (
	"fmt"
	"time"

	"github.com/aaronromeo/postmanpat/analysis"
)

// Decision represents a rule generation decision stored in SQLite
type Decision struct {
	ID          int64     `json:"id"`
	Lens        string    `json:"lens"` // list_lens, sender_unsub_lens, recipient_tag_lens
	ClusterID   string    `json:"cluster_id"`
	Type        string    `json:"type"`        // ignore, watch, cleanup
	Action      string    `json:"action"`      // delete, move
	Destination string    `json:"destination"` // for move action
	AgeWindow   string    `json:"age_window"`  // e.g., "30d", "7d", "1h" (only for cleanup)
	CreatedAt   time.Time `json:"created_at"`
}

// Validate checks if the decision is valid before storing
func (d *Decision) Validate() error {
	if d.Lens == "" {
		return fmt.Errorf("lens is required")
	}
	if d.ClusterID == "" {
		return fmt.Errorf("cluster_id is required")
	}
	if d.Type != "ignore" && d.Type != "watch" && d.Type != "cleanup" {
		return fmt.Errorf("type must be ignore, watch, or cleanup")
	}
	if d.Type != "ignore" {
		// watch and cleanup require action
		if d.Action != "delete" && d.Action != "move" {
			return fmt.Errorf("action must be delete or move")
		}
		if d.Action == "move" && d.Destination == "" {
			return fmt.Errorf("destination is required for move action")
		}
		if d.Type == "cleanup" && d.AgeWindow == "" {
			return fmt.Errorf("age_window is required for cleanup")
		}
	}
	return nil
}

// ClusterView represents a cluster for UI display
type ClusterView struct {
	ClusterID   string                   `json:"cluster_id"`
	Lens        string                   `json:"lens"`
	Count       int                      `json:"count"`
	Keys        map[string]interface{}   `json:"keys"`
	Examples    analysis.ClusterExamples `json:"examples"`
	HasDecision bool                     `json:"has_decision"`
}
