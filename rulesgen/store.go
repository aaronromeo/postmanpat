package rulesgen

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("rulesgen store: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS clusters (
			cluster_id TEXT PRIMARY KEY,
			lens TEXT NOT NULL,
			keys_json TEXT NOT NULL,
			count INTEGER NOT NULL,
			latest_date TEXT NOT NULL DEFAULT '',
			examples_json TEXT NOT NULL,
			signals_json TEXT NOT NULL,
			suppressed_json TEXT NOT NULL DEFAULT '[]',
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS decisions (
			cluster_id TEXT NOT NULL,
			lane TEXT NOT NULL,
			decision TEXT NOT NULL,
			decided_at TEXT NOT NULL,
			payload TEXT,
			PRIMARY KEY (cluster_id, lane)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("rulesgen store: schema: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) UpsertClusters(clusters []Cluster) error {
	for _, c := range clusters {
		keys, err := json.Marshal(c.Keys)
		if err != nil {
			return fmt.Errorf("rulesgen store: marshal keys for %s: %w", c.ClusterID, err)
		}
		examples, err := json.Marshal(c.Examples)
		if err != nil {
			return fmt.Errorf("rulesgen store: marshal examples for %s: %w", c.ClusterID, err)
		}
		signals, err := json.Marshal(c.Signals)
		if err != nil {
			return fmt.Errorf("rulesgen store: marshal signals for %s: %w", c.ClusterID, err)
		}
		suppressed, err := json.Marshal(c.Suppressed)
		if err != nil {
			return fmt.Errorf("rulesgen store: marshal suppressed for %s: %w", c.ClusterID, err)
		}
		if _, err := s.db.Exec(`INSERT INTO clusters
			(cluster_id, lens, keys_json, count, latest_date, examples_json, signals_json, suppressed_json, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(cluster_id) DO UPDATE SET
				lens = excluded.lens,
				keys_json = excluded.keys_json,
				count = excluded.count,
				latest_date = excluded.latest_date,
				examples_json = excluded.examples_json,
				signals_json = excluded.signals_json,
				suppressed_json = excluded.suppressed_json,
				last_seen = excluded.last_seen`,
			c.ClusterID, c.Lens, string(keys), c.Count, c.LatestDate, string(examples), string(signals), string(suppressed), c.LastSeen, c.LastSeen,
		); err != nil {
			return fmt.Errorf("rulesgen store: upsert cluster %s: %w", c.ClusterID, err)
		}
	}
	return nil
}

func (s *Store) PendingClusters() ([]Cluster, error) {
	rows, err := s.db.Query(`SELECT cluster_id, lens, keys_json, count, latest_date, examples_json, signals_json, suppressed_json, first_seen, last_seen
		FROM clusters`)
	if err != nil {
		return nil, fmt.Errorf("rulesgen store: query clusters: %w", err)
	}
	var clusters []Cluster
	for rows.Next() {
		var c Cluster
		var keys, examples, signals, suppressed string
		if err := rows.Scan(&c.ClusterID, &c.Lens, &keys, &c.Count, &c.LatestDate, &examples, &signals, &suppressed, &c.FirstSeen, &c.LastSeen); err != nil {
			rows.Close()
			return nil, fmt.Errorf("rulesgen store: scan cluster: %w", err)
		}
		if err := json.Unmarshal([]byte(keys), &c.Keys); err != nil {
			rows.Close()
			return nil, fmt.Errorf("rulesgen store: unmarshal keys for %s: %w", c.ClusterID, err)
		}
		if err := json.Unmarshal([]byte(examples), &c.Examples); err != nil {
			rows.Close()
			return nil, fmt.Errorf("rulesgen store: unmarshal examples for %s: %w", c.ClusterID, err)
		}
		if err := json.Unmarshal([]byte(signals), &c.Signals); err != nil {
			rows.Close()
			return nil, fmt.Errorf("rulesgen store: unmarshal signals for %s: %w", c.ClusterID, err)
		}
		if err := json.Unmarshal([]byte(suppressed), &c.Suppressed); err != nil {
			rows.Close()
			return nil, fmt.Errorf("rulesgen store: unmarshal suppressed for %s: %w", c.ClusterID, err)
		}
		clusters = append(clusters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rulesgen store: iterate clusters: %w", err)
	}

	decided, err := s.decidedLanes()
	if err != nil {
		return nil, err
	}
	pending := make([]Cluster, 0, len(clusters))
	for _, c := range clusters {
		if c.suppressedForBoth() {
			continue
		}
		if hasUndecidedLane(c, decided[c.ClusterID]) {
			pending = append(pending, c)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].LastSeen > pending[j].LastSeen
	})
	return pending, nil
}

func (s *Store) decidedLanes() (map[string]map[Lane]bool, error) {
	rows, err := s.db.Query(`SELECT cluster_id, lane FROM decisions`)
	if err != nil {
		return nil, fmt.Errorf("rulesgen store: query decisions: %w", err)
	}
	defer rows.Close()
	decided := make(map[string]map[Lane]bool)
	for rows.Next() {
		var clusterID string
		var lane Lane
		if err := rows.Scan(&clusterID, &lane); err != nil {
			return nil, fmt.Errorf("rulesgen store: scan decision: %w", err)
		}
		if decided[clusterID] == nil {
			decided[clusterID] = make(map[Lane]bool)
		}
		decided[clusterID][lane] = true
	}
	return decided, rows.Err()
}

func hasUndecidedLane(c Cluster, decided map[Lane]bool) bool {
	for _, lane := range lensLanes[c.Lens] {
		if !decided[lane] {
			return true
		}
	}
	return false
}
