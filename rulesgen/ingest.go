package rulesgen

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func IngestDir(dir string, st *Store) error {
	files, err := filepath.Glob(filepath.Join(dir, "postmanpat-analyze-*.json"))
	if err != nil {
		return fmt.Errorf("rulesgen ingest: glob %s: %w", dir, err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			log.Printf("rulesgen ingest: skipping %s: %v", file, err)
			continue
		}
		var rep report
		if err := json.Unmarshal(data, &rep); err != nil {
			log.Printf("rulesgen ingest: skipping %s: %v", file, err)
			continue
		}
		if err := st.UpsertClusters(rep.clusters()); err != nil {
			return err
		}
	}
	return nil
}
