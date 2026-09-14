package rulesgen

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Decision string

const (
	DecisionGenerated Decision = "generated"
	DecisionDeclined  Decision = "declined"
	DecisionIgnored   Decision = "ignored"
	DecisionSnoozed   Decision = "snoozed"
)

// StoredDecision is one (cluster, lane) row. Payload carries the JSON rules
// for generated, the ADR 0002 ignore identity for ignored, and nothing for
// declined/snoozed.
type StoredDecision struct {
	ClusterID string
	Lane      Lane
	Decision  Decision
	DecidedAt string
	Payload   []byte
}

// DecidedCluster pairs a cluster with every decision row attached to it.
type DecidedCluster struct {
	Cluster   Cluster
	Decisions []StoredDecision
}

func (s *Store) Decide(clusterID string, lane Lane, decision Decision, payload []byte) error {
	if _, err := s.db.Exec(`INSERT INTO decisions (cluster_id, lane, decision, decided_at, payload)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(cluster_id, lane) DO UPDATE SET
			decision = excluded.decision,
			decided_at = excluded.decided_at,
			payload = excluded.payload`,
		clusterID, lane, decision, time.Now().UTC().Format(time.RFC3339), string(payload),
	); err != nil {
		return fmt.Errorf("rulesgen store: decide %s:%s: %w", clusterID, lane, err)
	}
	return nil
}

func (s *Store) Revert(clusterID string, lane Lane) error {
	if _, err := s.db.Exec(`DELETE FROM decisions WHERE cluster_id = ? AND lane = ?`, clusterID, lane); err != nil {
		return fmt.Errorf("rulesgen store: revert %s:%s: %w", clusterID, lane, err)
	}
	return nil
}

// clearSnoozed removes snoozed decision rows for the given cluster IDs, but
// only when the snooze predates the boundary: the report generator passes its
// generated_at, so re-ingesting an unchanged (older) report never resurrects a
// snoozed cluster while a fresh report containing it clears the snooze.
func (s *Store) clearSnoozed(clusterIDs []string, newerThan string) error {
	stmt, err := s.db.Prepare(`DELETE FROM decisions
		WHERE cluster_id = ? AND lane = ? AND decision = ? AND decided_at < ?`)
	if err != nil {
		return fmt.Errorf("rulesgen store: prepare clearSnoozed: %w", err)
	}
	defer stmt.Close()
	for _, id := range clusterIDs {
		for _, lane := range []Lane{LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup} {
			if _, err := stmt.Exec(id, lane, DecisionSnoozed, newerThan); err != nil {
				return fmt.Errorf("rulesgen store: clear snooze %s:%s: %w", id, lane, err)
			}
		}
	}
	return nil
}

func (s *Store) AllDecisions() ([]StoredDecision, error) {
	rows, err := s.db.Query(`SELECT cluster_id, lane, decision, decided_at, payload
		FROM decisions ORDER BY cluster_id, lane`)
	if err != nil {
		return nil, fmt.Errorf("rulesgen store: query decisions: %w", err)
	}
	defer rows.Close()
	var out []StoredDecision
	for rows.Next() {
		var d StoredDecision
		var payload sql.NullString
		if err := rows.Scan(&d.ClusterID, &d.Lane, &d.Decision, &d.DecidedAt, &payload); err != nil {
			return nil, fmt.Errorf("rulesgen store: scan decision: %w", err)
		}
		if payload.Valid {
			d.Payload = []byte(payload.String)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDecisions returns clusters whose offered lanes are all decided, paired
// with their decisions, ordered by cluster ID. Clusters with pending lanes are
// omitted; decisions whose cluster has vanished are ignored.
func (s *Store) ListDecisions() ([]DecidedCluster, error) {
	decided, err := s.decidedLanes()
	if err != nil {
		return nil, err
	}
	byCluster := map[string][]StoredDecision{}
	{
		all, err := s.AllDecisions()
		if err != nil {
			return nil, err
		}
		for _, d := range all {
			byCluster[d.ClusterID] = append(byCluster[d.ClusterID], d)
		}
	}
	var out []DecidedCluster
	for clusterID, done := range decided {
		c, err := s.ClusterByID(clusterID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if hasUndecidedLane(c, done) {
			continue
		}
		out = append(out, DecidedCluster{Cluster: c, Decisions: byCluster[clusterID]})
	}
	sortDecidedClusters(out)
	return out, nil
}
