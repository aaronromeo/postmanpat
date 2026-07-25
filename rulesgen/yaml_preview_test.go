package rulesgen

import (
	"testing"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRulePreview(t *testing.T) {
	// Test cluster for list lens
	listCluster := analysis.Cluster{
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

	// Test cluster for sender lens
	senderCluster := analysis.Cluster{
		ClusterID: "sender_unsub_lens:sender456",
		Count:     5,
		Keys: map[string]any{
			"SenderDomain": "example.com",
		},
		Examples: analysis.ClusterExamples{
			SubjectRaw:        []string{"Your weekly update"},
			SenderDomains:     []string{"newsletter@example.com", "updates@example.com"},
			ReplyToDomains:    []string{"noreply@example.com"},
			ListUnsubscribeTargets: []string{"mailto:unsubscribe@example.com"},
		},
		Signals: analysis.ClusterSignals{
			HasListUnsubscribe: true,
		},
	}

	t.Run("ignore decision", func(t *testing.T) {
		preview, err := GenerateRulePreview(listCluster, "ignore", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, "", preview.WatchRule)
		assert.Equal(t, "", preview.CleanupRule)
		assert.Equal(t, "", preview.OnetimeRule)
		assert.Equal(t, "", preview.ErrorMessage)
	})

	t.Run("watch decision for list lens", func(t *testing.T) {
		preview, err := GenerateRulePreview(listCluster, "watch", "delete", "", "")
		require.NoError(t, err)
		assert.NotEmpty(t, preview.WatchRule)
		assert.Contains(t, preview.WatchRule, "list_id_regex")
		assert.Contains(t, preview.WatchRule, "example-list@lists\\.example\\.com")
		assert.Contains(t, preview.WatchRule, "delete")
		assert.Equal(t, "", preview.CleanupRule)
		assert.Equal(t, "", preview.OnetimeRule)
	})

	t.Run("watch decision with move action", func(t *testing.T) {
		preview, err := GenerateRulePreview(listCluster, "watch", "move", "Newsletters", "")
		require.NoError(t, err)
		assert.Contains(t, preview.WatchRule, "move")
		assert.Contains(t, preview.WatchRule, "Newsletters")
	})

	t.Run("cleanup decision for sender lens", func(t *testing.T) {
		preview, err := GenerateRulePreview(senderCluster, "cleanup", "delete", "", "30d")
		require.NoError(t, err)
		assert.NotEmpty(t, preview.CleanupRule)
		assert.Contains(t, preview.CleanupRule, "age_window")
		assert.Contains(t, preview.CleanupRule, "30d")
		assert.Contains(t, preview.CleanupRule, "sender_substring")
		assert.Contains(t, preview.CleanupRule, "example.com")
		assert.Contains(t, preview.CleanupRule, "list_unsubscribe")
	})

	t.Run("cleanup decision generates onetime rule", func(t *testing.T) {
		preview, err := GenerateRulePreview(senderCluster, "cleanup", "move", "Archive", "7d")
		require.NoError(t, err)
		assert.Contains(t, preview.CleanupRule, "age_window")
		assert.Contains(t, preview.OnetimeRule, "move")
		assert.Contains(t, preview.OnetimeRule, "Archive")
		// Onetime rule should NOT have age_window (or at least not the same age_window)
		if preview.OnetimeRule != "" {
			assert.NotContains(t, preview.OnetimeRule, "min: \"7d\"")
		}
	})

	t.Run("recipient tag lens watch rule", func(t *testing.T) {
		recipientCluster := analysis.Cluster{
			ClusterID: "recipient_tag_lens:tag789",
			Count:     3,
			Keys: map[string]any{
				"recipient_tag": "newsletter",
			},
			Examples: analysis.ClusterExamples{
				SubjectRaw: []string{"Newsletter"},
			},
		}

		preview, err := GenerateRulePreview(recipientCluster, "watch", "delete", "", "")
		require.NoError(t, err)
		assert.Contains(t, preview.WatchRule, "recipient_tag_regex")
		assert.Contains(t, preview.WatchRule, "newsletter")
	})

	t.Run("recipient tag lens cleanup not supported", func(t *testing.T) {
		recipientCluster := analysis.Cluster{
			ClusterID: "recipient_tag_lens:tag789",
			Count:     3,
			Keys: map[string]any{
				"recipient_tag": "newsletter",
			},
		}

		preview, err := GenerateRulePreview(recipientCluster, "cleanup", "delete", "", "30d")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
		assert.Equal(t, "", preview.CleanupRule)
	})

	t.Run("invalid cluster ID format", func(t *testing.T) {
		invalidCluster := analysis.Cluster{
			ClusterID: "no-colon",
			Count:     1,
		}

		preview, err := GenerateRulePreview(invalidCluster, "watch", "delete", "", "")
		// This should error because no suitable matchers found
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no suitable matchers found")
		assert.Equal(t, "", preview.WatchRule)
	})

	t.Run("invalid decision type", func(t *testing.T) {
		preview, err := GenerateRulePreview(listCluster, "invalid", "delete", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown decision type")
		assert.Equal(t, "", preview.WatchRule)
	})

	t.Run("regex escaping", func(t *testing.T) {
		cluster := analysis.Cluster{
			ClusterID: "list_lens:test",
			Count:     1,
			Keys: map[string]any{
				"ListID": "test.example.com+special(chars)*",
			},
		}

		preview, err := GenerateRulePreview(cluster, "watch", "delete", "", "")
		require.NoError(t, err)
		// Check that special regex chars are escaped
		assert.Contains(t, preview.WatchRule, `test\.example\.com\+special\(chars\)\*`)
	})
}

func TestRegexEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test.example.com", "test\\.example\\.com"},
		{"test+plus", "test\\+plus"},
		{"test*star", "test\\*star"},
		{"test?question", "test\\?question"},
		{"test(paren)", "test\\(paren\\)"},
		{"test[ bracket ]", "test\\[ bracket \\]"},
		{"normal", "normal"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := regexEscape(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSubjectPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Re: Hello World", "Hello World"},
		{"Fwd: Important Message!", "Important Message"},
		{"[LIST] Newsletter", "Newsletter"},
		{"Normal Subject", "Normal Subject"},
		{"Subject with trailing...", "Subject with trailing"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractSubjectPattern(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommonWords(t *testing.T) {
	assert.True(t, isCommonWord("the"))
	assert.True(t, isCommonWord("AND"))
	assert.True(t, isCommonWord("You"))
	assert.False(t, isCommonWord("newsletter"))
	assert.False(t, isCommonWord("example"))
	assert.False(t, isCommonWord(""))
}

func TestExtractCommonWords(t *testing.T) {
	result := extractCommonWords("The quick brown fox jumps over the lazy dog")
	// Should filter out "the" and other common words
	assert.NotContains(t, result, "the")
	assert.Contains(t, result, "quick")
	assert.Contains(t, result, "brown")
	assert.Contains(t, result, "fox")
}

func TestYAMLStructure(t *testing.T) {
	// Test that generated YAML is valid YAML structure
	cluster := analysis.Cluster{
		ClusterID: "list_lens:test123",
		Count:     10,
		Keys: map[string]any{
			"ListID": "test@example.com",
		},
		Examples: analysis.ClusterExamples{
			SubjectRaw: []string{"Test"},
		},
	}

	preview, err := GenerateRulePreview(cluster, "watch", "delete", "", "")
	require.NoError(t, err)
	
	// Basic YAML structure checks
	assert.Contains(t, preview.WatchRule, "rules:")
	assert.Contains(t, preview.WatchRule, "name:")
	assert.Contains(t, preview.WatchRule, "actions:")
	assert.Contains(t, preview.WatchRule, "client:")
	assert.Contains(t, preview.WatchRule, "list_id_regex")
}