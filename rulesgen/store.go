package rulesgen

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store interface {
	BeginTransaction() (*sql.Tx, error)
	Close() error
	CreateDecision(d Decision) (int64, error)
	CreateDecisionTx(tx *sql.Tx, d Decision) (int64, error)
	DeleteDecision(lens string, clusterID string, decisionType string) error
	GetAllDecidedClusterIDs() (map[string]bool, error)
	GetAllDecisions() ([]Decision, error)
	GetDecisions(lens string, clusterID string) ([]Decision, error)
	GetDecisionsByType(decisionType string) ([]Decision, error)
	HasDecision(lens string, clusterID string) (bool, error)
}

// SqlStore handles persistent storage of decisions using SQLite
type SqlStore struct {
	db *sql.DB
}

// NewStore creates a new SQLite store at the given path
func NewStore(dbPath string) (*SqlStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &SqlStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database tables and indexes
func (s *SqlStore) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS decisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lens TEXT NOT NULL,
    cluster_id TEXT NOT NULL,
    decision_type TEXT NOT NULL CHECK(decision_type IN ('ignore', 'watch', 'cleanup')),
    action TEXT,
    destination TEXT,
    age_window TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(lens, cluster_id, decision_type),
    CHECK((decision_type = 'ignore' AND (action IS NULL OR action = '')) OR 
          (decision_type IN ('watch', 'cleanup') AND action IN ('delete', 'move')))
);

CREATE INDEX IF NOT EXISTS idx_decisions_lookup ON decisions(lens, cluster_id);
`
	_, err := s.db.Exec(schema)
	return err
}

// CreateDecision inserts a new decision into the database
func (s *SqlStore) CreateDecision(d Decision) (int64, error) {
	query := `
INSERT INTO decisions (lens, cluster_id, decision_type, action, destination, age_window)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(lens, cluster_id, decision_type) DO UPDATE SET
    action = excluded.action,
    destination = excluded.destination,
    age_window = excluded.age_window,
    created_at = CURRENT_TIMESTAMP
`

	// Convert empty strings to NULL for SQLite
	action := sql.NullString{String: d.Action, Valid: d.Action != ""}
	destination := sql.NullString{String: d.Destination, Valid: d.Destination != ""}
	ageWindow := sql.NullString{String: d.AgeWindow, Valid: d.AgeWindow != ""}

	result, err := s.db.Exec(query, d.Lens, d.ClusterID, d.Type, action, destination, ageWindow)
	if err != nil {
		return 0, fmt.Errorf("failed to insert decision: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return id, nil
}

// GetDecisions retrieves all decisions for a specific cluster
func (s *SqlStore) GetDecisions(lens string, clusterID string) ([]Decision, error) {
	query := `
SELECT id, lens, cluster_id, decision_type, action, destination, age_window, created_at
FROM decisions
WHERE lens = ? AND cluster_id = ?
ORDER BY created_at DESC
`

	rows, err := s.db.Query(query, lens, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to query decisions: %w", err)
	}
	defer rows.Close()

	return s.scanDecisions(rows)
}

// HasDecision checks if any decision exists for a cluster
func (s *SqlStore) HasDecision(lens string, clusterID string) (bool, error) {
	query := `SELECT 1 FROM decisions WHERE lens = ? AND cluster_id = ? LIMIT 1`
	var exists int
	err := s.db.QueryRow(query, lens, clusterID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check decision: %w", err)
	}
	return true, nil
}

// GetAllDecisions retrieves all decisions from the database
func (s *SqlStore) GetAllDecisions() ([]Decision, error) {
	query := `
SELECT id, lens, cluster_id, decision_type, action, destination, age_window, created_at
FROM decisions
ORDER BY created_at DESC
`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all decisions: %w", err)
	}
	defer rows.Close()

	return s.scanDecisions(rows)
}

// GetDecisionsByType retrieves all decisions of a specific type
func (s *SqlStore) GetDecisionsByType(decisionType string) ([]Decision, error) {
	query := `
SELECT id, lens, cluster_id, decision_type, action, destination, age_window, created_at
FROM decisions
WHERE decision_type = ?
ORDER BY created_at DESC
`

	rows, err := s.db.Query(query, decisionType)
	if err != nil {
		return nil, fmt.Errorf("failed to query decisions by type: %w", err)
	}
	defer rows.Close()

	return s.scanDecisions(rows)
}

// GetAllDecidedClusterIDs returns a map of decided clusters for filtering
// The key is formatted as "lens:cluster_id"
func (s *SqlStore) GetAllDecidedClusterIDs() (map[string]bool, error) {
	query := `SELECT DISTINCT lens, cluster_id FROM decisions`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query decided clusters: %w", err)
	}
	defer rows.Close()

	decided := make(map[string]bool)
	for rows.Next() {
		var lens, clusterID string
		if err := rows.Scan(&lens, &clusterID); err != nil {
			return nil, fmt.Errorf("failed to scan decided cluster: %w", err)
		}
		key := fmt.Sprintf("%s:%s", lens, clusterID)
		decided[key] = true
	}

	return decided, rows.Err()
}

// DeleteDecision removes a decision from the database
func (s *SqlStore) DeleteDecision(lens string, clusterID string, decisionType string) error {
	query := `DELETE FROM decisions WHERE lens = ? AND cluster_id = ? AND decision_type = ?`
	_, err := s.db.Exec(query, lens, clusterID, decisionType)
	if err != nil {
		return fmt.Errorf("failed to delete decision: %w", err)
	}
	return nil
}

// Close closes the database connection
func (s *SqlStore) Close() error {
	return s.db.Close()
}

// scanDecisions is a helper to scan decision rows
func (s *SqlStore) scanDecisions(rows *sql.Rows) ([]Decision, error) {
	var decisions []Decision
	for rows.Next() {
		var d Decision
		var action, destination, ageWindow sql.NullString
		var createdAt sql.NullTime
		err := rows.Scan(
			&d.ID,
			&d.Lens,
			&d.ClusterID,
			&d.Type,
			&action,
			&destination,
			&ageWindow,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan decision: %w", err)
		}
		if action.Valid {
			d.Action = action.String
		}
		if destination.Valid {
			d.Destination = destination.String
		}
		if ageWindow.Valid {
			d.AgeWindow = ageWindow.String
		}
		if createdAt.Valid {
			d.CreatedAt = createdAt.Time
		}
		decisions = append(decisions, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return decisions, nil
}

// BeginTransaction starts a new transaction
func (s *SqlStore) BeginTransaction() (*sql.Tx, error) {
	return s.db.Begin()
}

// CreateDecisionTx creates a decision within a transaction
func (s *SqlStore) CreateDecisionTx(tx *sql.Tx, d Decision) (int64, error) {
	query := `
INSERT INTO decisions (lens, cluster_id, decision_type, action, destination, age_window)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(lens, cluster_id, decision_type) DO UPDATE SET
    action = excluded.action,
    destination = excluded.destination,
    age_window = excluded.age_window,
    created_at = CURRENT_TIMESTAMP
`

	// Convert empty strings to NULL for SQLite
	action := sql.NullString{String: d.Action, Valid: d.Action != ""}
	destination := sql.NullString{String: d.Destination, Valid: d.Destination != ""}
	ageWindow := sql.NullString{String: d.AgeWindow, Valid: d.AgeWindow != ""}

	result, err := tx.Exec(query, d.Lens, d.ClusterID, d.Type, action, destination, ageWindow)
	if err != nil {
		return 0, fmt.Errorf("failed to insert decision in transaction: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return id, nil
}
