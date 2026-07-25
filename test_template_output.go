package main

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/aaronromeo/postmanpat/rulesgen"
)

func main() {
	// Create template with same function map as server
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"js": func(s string) string {
			return strings.ReplaceAll(s, ":", "_")
		},
	}).ParseFiles("rulesgen/templates/index.html")
	if err != nil {
		fmt.Printf("Error parsing template: %v\n", err)
		return
	}

	// Create test data with YAML snippets
	cluster := analysis.Cluster{
		ClusterID: "list_lens:test123",
		Count:     10,
		Keys: map[string]any{
			"ListID": "example-list@lists.example.com",
		},
		Examples: analysis.ClusterExamples{
			SubjectRaw: []string{"Monthly Newsletter"},
		},
		Signals: analysis.ClusterSignals{
			HasListID:          true,
			HasListUnsubscribe: true,
		},
	}

	// Generate YAML previews
	watchPreview, _ := rulesgen.GenerateRulePreview(cluster, "watch", "delete", "", "")
	cleanupPreview, _ := rulesgen.GenerateRulePreview(cluster, "cleanup", "delete", "", "30d")

	view := rulesgen.ClusterView{
		ClusterID:   cluster.ClusterID,
		Lens:        "list_lens",
		Count:       cluster.Count,
		Keys:        cluster.Keys,
		Examples:    cluster.Examples,
		HasDecision: false,
		WatchYAML:   watchPreview.WatchRule,
		CleanupYAML: cleanupPreview.CleanupRule,
	}

	data := struct {
		Clusters []rulesgen.ClusterView
		Total    int
	}{
		Clusters: []rulesgen.ClusterView{view},
		Total:    1,
	}

	// Execute template
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		fmt.Printf("Error executing template: %v\n", err)
		return
	}

	// Check output
	html := buf.String()
	
	// Look for key elements
	fmt.Println("=== Checking template output ===")
	fmt.Printf("Contains YAML preview sections: %v\n", strings.Contains(html, "yaml-preview"))
	fmt.Printf("Contains Watch rule: %v\n", strings.Contains(html, "Watch Rule"))
	fmt.Printf("Contains Cleanup rule: %v\n", strings.Contains(html, "Cleanup Rule"))
	fmt.Printf("Contains cluster ID: %v\n", strings.Contains(html, "test123"))
	fmt.Printf("Contains list_id_regex: %v\n", strings.Contains(html, "list_id_regex"))
	
	// Show a snippet
	if len(html) > 500 {
		fmt.Println("\n=== Sample output (first 500 chars) ===")
		fmt.Println(html[:500])
	}
}