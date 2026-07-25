package rulesgen

import (
	"testing"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/stretchr/testify/assert"
)

func TestConvertLensToViews(t *testing.T) {
	// Create a test lens with some clusters
	lens := analysis.Lens{
		Clusters: []analysis.Cluster{
			{
				ClusterID: "list_lens:test123",
				Count:     10,
				Examples: analysis.ClusterExamples{
					SubjectRaw: []string{"Test Subject"},
				},
			},
			{
				ClusterID: "sender_unsub_lens:test456",
				Count:     5,
				Examples: analysis.ClusterExamples{
					SenderDomains: []string{"example.com"},
				},
			},
		},
	}
	
	// Test with empty decided clusters
	decided := make(map[string]bool)
	views := convertLensToViews(lens, decided)
	
	assert.Len(t, views, 2)
	assert.Equal(t, "list_lens:test123", views[0].ClusterID)
	assert.Equal(t, "list_lens", views[0].Lens)
	assert.Equal(t, 10, views[0].Count)
	assert.False(t, views[0].HasDecision)
	
	// Test with one decided cluster
	decided["list_lens:test123"] = true
	views = convertLensToViews(lens, decided)
	
	assert.Len(t, views, 1, "should filter out decided cluster")
	assert.Equal(t, "sender_unsub_lens:test456", views[0].ClusterID)
}

func TestExtractLensFromClusterID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"list_lens:abc123", "list_lens"},
		{"sender_unsub_lens:def456", "sender_unsub_lens"},
		{"recipient_tag_lens:ghi789", "recipient_tag_lens"},
		{"invalid", ""},
		{"", ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractLensFromClusterID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}