package rulesgen

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateMobileFriendly(t *testing.T) {
	// Parse the template with same function map as server
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"js": func(s string) string {
			return strings.ReplaceAll(s, ":", "_")
		},
	}).ParseFS(templatesFS, "templates/*.html")
	require.NoError(t, err, "should parse template")
	
	// Create test data
	data := struct {
		Clusters []ClusterView
		Total    int
	}{
		Clusters: []ClusterView{
			{
				ClusterID: "list_lens:test123",
				Lens:      "list_lens",
				Count:     10,
				Examples: analysis.ClusterExamples{
					SubjectRaw:        []string{"Welcome to our service", "Monthly newsletter"},
					SenderDomains:     []string{"example.com", "newsletter.example.com"},
					ListUnsubscribeTargets: []string{"mailto:unsubscribe@example.com"},
				},
			},
			{
				ClusterID: "sender_unsub_lens:test456",
				Lens:      "sender_unsub_lens",
				Count:     5,
				Examples: analysis.ClusterExamples{
					SubjectRaw:    []string{"Your order confirmation"},
					SenderDomains: []string{"store.example.com"},
				},
			},
		},
		Total: 2,
	}
	
	// Execute template
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "index.html", data)
	require.NoError(t, err, "should execute template")
	
	html := buf.String()
	
	// Test for mobile-friendly features
	t.Run("viewport meta tag", func(t *testing.T) {
		assert.Contains(t, html, "viewport", "should contain viewport meta tag")
		assert.Contains(t, html, "width=device-width", "should have responsive width")
		assert.Contains(t, html, "initial-scale=1.0", "should have proper initial scale")
	})
	
	t.Run("responsive CSS", func(t *testing.T) {
		assert.Contains(t, html, "@media", "should contain media queries")
		assert.Contains(t, html, "max-width", "should contain max-width constraints")
		assert.Contains(t, html, "min-width", "should contain min-width for table")
	})
	
	t.Run("touch-friendly classes", func(t *testing.T) {
		assert.Contains(t, html, "table-container", "should have table container for scrolling")
		assert.Contains(t, html, "-webkit-overflow-scrolling", "should have smooth scrolling for iOS")
		assert.Contains(t, html, "yaml-preview", "should have YAML preview sections")
	})
	
	t.Run("semantic structure", func(t *testing.T) {
		assert.Contains(t, html, "<!DOCTYPE html>", "should have proper doctype")
		assert.Contains(t, html, "<head>", "should have head section")
		assert.Contains(t, html, "<body>", "should have body section")
		assert.Contains(t, html, "class=\"container\"", "should have container class")
	})
	
	t.Run("cluster display", func(t *testing.T) {
		assert.Contains(t, html, "test123", "should display cluster ID")
		assert.Contains(t, html, "list_lens", "should display lens type")
		assert.Contains(t, html, "10", "should display count")
		assert.Contains(t, html, "Welcome to our service", "should display example subject")
	})
	
	t.Run("empty state", func(t *testing.T) {
		// Test with empty clusters
		emptyData := struct {
			Clusters []ClusterView
			Total    int
		}{
			Clusters: []ClusterView{},
			Total:    0,
		}
		
		var emptyBuf bytes.Buffer
		err = tmpl.ExecuteTemplate(&emptyBuf, "index.html", emptyData)
		require.NoError(t, err, "should execute template with empty data")
		
		emptyHTML := emptyBuf.String()
		assert.Contains(t, emptyHTML, "No clusters to review", "should show empty state message")
		assert.Contains(t, emptyHTML, "Analysis complete", "should show status badge")
	})
}

func TestTemplateAccessibility(t *testing.T) {
	// Parse the template with same function map as server
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"js": func(s string) string {
			return strings.ReplaceAll(s, ":", "_")
		},
	}).ParseFS(templatesFS, "templates/*.html")
	require.NoError(t, err)
	
	data := struct {
		Clusters []ClusterView
		Total    int
	}{
		Clusters: []ClusterView{
			{
				ClusterID: "test:123",
				Lens:      "list_lens",
				Count:     1,
			},
		},
		Total: 1,
	}
	
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "index.html", data)
	require.NoError(t, err)
	
	html := buf.String()
	
	// Basic accessibility checks
	assert.False(t, strings.Contains(strings.ToLower(html), "<font "), "should avoid deprecated font tags")
	assert.False(t, strings.Contains(strings.ToLower(html), "<center>"), "should avoid deprecated center tags")
	assert.True(t, strings.Contains(html, "role=\"") || !strings.Contains(html, "role="), "should use semantic HTML or proper ARIA roles if needed")
	
	// Table structure
	assert.Contains(t, html, "<thead>", "should use table header for accessibility")
	assert.Contains(t, html, "<tbody>", "should use table body for accessibility")
}