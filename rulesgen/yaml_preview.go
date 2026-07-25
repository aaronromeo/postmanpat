package rulesgen

import (
	"fmt"
	"strings"

	"github.com/aaronromeo/postmanpat/analysis"
	"gopkg.in/yaml.v3"
)

// YAMLPreviewConfig holds configuration for generating YAML previews
type YAMLPreviewConfig struct {
	// Add any config needed for YAML generation
}

// RulePreview represents a preview of a rule that would be generated
type RulePreview struct {
	WatchRule    string `json:"watch_rule"`
	CleanupRule  string `json:"cleanup_rule"`
	OnetimeRule  string `json:"onetime_rule"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// GenerateRulePreview generates YAML previews for a cluster
func GenerateRulePreview(cluster analysis.Cluster, decisionType, action, destination, ageWindow string) (RulePreview, error) {
	var preview RulePreview
	
	// Extract lens and cluster ID
	lensName := extractLensFromClusterID(cluster.ClusterID)
	clusterID := extractClusterIDWithoutLens(cluster.ClusterID)
	
	// Generate rule name
	ruleName := fmt.Sprintf("%s_%s", lensName, clusterID)
	if len(ruleName) > 50 {
		ruleName = ruleName[:50]
	}
	
	switch decisionType {
	case "ignore":
		// No YAML generated for ignore decisions
		return preview, nil
		
	case "watch":
		// Generate watch rule (client-side matchers)
		watchRule, err := generateWatchRule(ruleName, cluster, action, destination)
		if err != nil {
			return preview, err
		}
		preview.WatchRule = watchRule
		
	case "cleanup":
		// Generate cleanup rule (server-side matchers with age window)
		cleanupRule, err := generateCleanupRule(ruleName, cluster, action, destination, ageWindow)
		if err != nil {
			return preview, err
		}
		preview.CleanupRule = cleanupRule
		
		// Also generate onetime rule (without age window)
		onetimeRule, err := generateCleanupRule(ruleName, cluster, action, destination, "")
		if err != nil {
			return preview, err
		}
		preview.OnetimeRule = onetimeRule
		
	default:
		return preview, fmt.Errorf("unknown decision type: %s", decisionType)
	}
	
	return preview, nil
}

// generateWatchRule creates a watch rule YAML (client-side regex matchers)
func generateWatchRule(ruleName string, cluster analysis.Cluster, action, destination string) (string, error) {
	rule := map[string]interface{}{
		"name": ruleName,
		"actions": []map[string]interface{}{
			{
				"type": action,
			},
		},
		"client": map[string]interface{}{},
	}
	
	// Add destination for move action
	if action == "move" && destination != "" {
		rule["actions"].([]map[string]interface{})[0]["destination"] = destination
	}
	
	// Add matchers based on lens type
	lensName := extractLensFromClusterID(cluster.ClusterID)
	
	switch lensName {
	case "list_lens":
		// Check for ListID in keys
		if listID, ok := cluster.Keys["ListID"]; ok {
			if listIDStr, ok := listID.(string); ok && listIDStr != "" {
				// Escape special regex characters
				escaped := regexEscape(listIDStr)
				rule["client"].(map[string]interface{})["list_id_regex"] = []string{fmt.Sprintf("(?i)%s", escaped)}
			}
		}
		
	case "sender_unsub_lens":
		// Use sender domains from examples
		if len(cluster.Examples.SenderDomains) > 0 {
			var regexes []string
			for _, domain := range cluster.Examples.SenderDomains {
				if domain != "" {
					escaped := regexEscape(domain)
					regexes = append(regexes, fmt.Sprintf("(?i)%s", escaped))
				}
			}
			if len(regexes) > 0 {
				rule["client"].(map[string]interface{})["sender_regex"] = regexes
			}
		}
		
		// Also add reply-to domains if available
		if len(cluster.Examples.ReplyToDomains) > 0 {
			var regexes []string
			for _, domain := range cluster.Examples.ReplyToDomains {
				if domain != "" {
					escaped := regexEscape(domain)
					regexes = append(regexes, fmt.Sprintf("(?i)%s", escaped))
				}
			}
			if len(regexes) > 0 {
				rule["client"].(map[string]interface{})["replyto_regex"] = regexes
			}
		}
		
		// Check for list unsubscribe signals
		if cluster.Signals.HasListUnsubscribe && len(cluster.Examples.ListUnsubscribeTargets) > 0 {
			listUnsubscribe := true
			rule["client"].(map[string]interface{})["list_unsubscribe"] = &listUnsubscribe
		}
		
	case "recipient_tag_lens":
		// Use recipient tag from keys
		if recipientTag, ok := cluster.Keys["recipient_tag"]; ok {
			if tagStr, ok := recipientTag.(string); ok && tagStr != "" {
				escaped := regexEscape(tagStr)
				rule["client"].(map[string]interface{})["recipient_tag_regex"] = []string{fmt.Sprintf("(?i)%s", escaped)}
			}
		}
		
	default:
		// For template lens or unknown, use subject patterns
		if len(cluster.Examples.SubjectRaw) > 0 {
			var regexes []string
			for _, subject := range cluster.Examples.SubjectRaw {
				if subject != "" {
					// Extract common patterns from subjects
					pattern := extractSubjectPattern(subject)
					if pattern != "" {
						escaped := regexEscape(pattern)
						regexes = append(regexes, fmt.Sprintf("(?i)%s", escaped))
					}
				}
			}
			if len(regexes) > 0 {
				rule["client"].(map[string]interface{})["subject_regex"] = regexes
			}
		}
	}
	
	// If no matchers were added, return error
	clientMatchers := rule["client"].(map[string]interface{})
	if len(clientMatchers) == 0 {
		return "", fmt.Errorf("no suitable matchers found for watch rule generation")
	}
	
	// Convert to YAML
	yamlBytes, err := yaml.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{rule},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}
	
	return string(yamlBytes), nil
}

// generateCleanupRule creates a cleanup rule YAML (server-side substring matchers)
func generateCleanupRule(ruleName string, cluster analysis.Cluster, action, destination, ageWindow string) (string, error) {
	rule := map[string]interface{}{
		"name": ruleName,
		"actions": []map[string]interface{}{
			{
				"type": action,
			},
		},
		"server": map[string]interface{}{
			"folders": []string{"INBOX"},
		},
	}
	
	// Add destination for move action
	if action == "move" && destination != "" {
		rule["actions"].([]map[string]interface{})[0]["destination"] = destination
	}
	
	// Add age window for cleanup rules (not for onetime)
	if ageWindow != "" {
		rule["server"].(map[string]interface{})["age_window"] = map[string]interface{}{
			"min": ageWindow,
		}
	}
	
	// Add matchers based on lens type
	lensName := extractLensFromClusterID(cluster.ClusterID)
	
	switch lensName {
	case "list_lens":
		// Check for ListID in keys
		if listID, ok := cluster.Keys["ListID"]; ok {
			if listIDStr, ok := listID.(string); ok && listIDStr != "" {
				rule["server"].(map[string]interface{})["list_id_substring"] = []string{listIDStr}
			}
		}
		
	case "sender_unsub_lens":
		// Use sender domains from examples
		if len(cluster.Examples.SenderDomains) > 0 {
			var substrings []string
			for _, domain := range cluster.Examples.SenderDomains {
				if domain != "" {
					substrings = append(substrings, domain)
				}
			}
			if len(substrings) > 0 {
				rule["server"].(map[string]interface{})["sender_substring"] = substrings
			}
		}
		
		// Also add reply-to domains if available
		if len(cluster.Examples.ReplyToDomains) > 0 {
			var substrings []string
			for _, domain := range cluster.Examples.ReplyToDomains {
				if domain != "" {
					substrings = append(substrings, domain)
				}
			}
			if len(substrings) > 0 {
				rule["server"].(map[string]interface{})["replyto_substring"] = substrings
			}
		}
		
		// Check for list unsubscribe signals
		if cluster.Signals.HasListUnsubscribe && len(cluster.Examples.ListUnsubscribeTargets) > 0 {
			listUnsubscribe := true
			rule["server"].(map[string]interface{})["list_unsubscribe"] = &listUnsubscribe
		}
		
	case "recipient_tag_lens":
		// Cleanup rules not supported for recipient_tag_lens
		return "", fmt.Errorf("cleanup rules not supported for recipient_tag_lens")
		
	default:
		// For template lens or unknown, use subject patterns
		if len(cluster.Examples.SubjectRaw) > 0 {
			var substrings []string
			for _, subject := range cluster.Examples.SubjectRaw {
				if subject != "" {
					// Extract common words from subjects
					words := extractCommonWords(subject)
					for _, word := range words {
						if len(word) > 3 { // Only use words longer than 3 chars
							substrings = append(substrings, word)
						}
					}
				}
			}
			if len(substrings) > 0 {
				rule["server"].(map[string]interface{})["body_substring"] = substrings
			}
		}
	}
	
	// If no matchers were added, return error
	serverMatchers := rule["server"].(map[string]interface{})
	if len(serverMatchers) <= 1 { // Only has "folders" field
		return "", fmt.Errorf("no suitable matchers found for cleanup rule generation")
	}
	
	// Convert to YAML
	yamlBytes, err := yaml.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{rule},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}
	
	return string(yamlBytes), nil
}

// regexEscape escapes special regex characters
func regexEscape(s string) string {
	specialChars := `\.+*?()|[]{}^$`
	var result strings.Builder
	for _, r := range s {
		if strings.ContainsRune(specialChars, r) {
			result.WriteRune('\\')
		}
		result.WriteRune(r)
	}
	return result.String()
}

// extractSubjectPattern extracts a pattern from a subject for regex matching
func extractSubjectPattern(subject string) string {
	subject = strings.TrimSpace(subject)
	
	// Remove common prefixes
	prefixes := []string{"Re:", "Fwd:", "FW:", "RE:", "fwd:", "re:"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(subject), strings.ToLower(prefix)) {
			subject = subject[len(prefix):]
			subject = strings.TrimSpace(subject)
		}
	}
	
	// Remove bracketed prefixes like [LIST], (Important), etc.
	if len(subject) > 0 && (subject[0] == '[' || subject[0] == '(') {
		// Find matching closing bracket
		closeChar := byte(']')
		if subject[0] == '(' {
			closeChar = byte(')')
		}
		
		closeIdx := strings.IndexByte(subject, closeChar)
		if closeIdx > 0 {
			subject = subject[closeIdx+1:]
			subject = strings.TrimSpace(subject)
		}
	}
	
	// Remove trailing punctuation
	subject = strings.TrimRight(subject, "!?.:;,- ")
	
	return subject
}

// extractCommonWords extracts common words from a string
func extractCommonWords(s string) []string {
	// Split by common separators
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == ':' || r == ';' || r == ','
	})
	
	var commonWords []string
	for _, word := range words {
		word = strings.Trim(word, "()[]{}!?.\"'")
		if len(word) > 0 && !isCommonWord(word) {
			commonWords = append(commonWords, strings.ToLower(word))
		}
	}
	
	return commonWords
}

// isCommonWord checks if a word is too common to be useful
func isCommonWord(word string) bool {
	commonWords := map[string]bool{
		"the": true, "and": true, "you": true, "for": true, "are": true,
		"with": true, "this": true, "that": true, "have": true, "from": true,
		"your": true, "will": true, "not": true, "but": true, "was": true,
		"all": true, "can": true, "out": true, "get": true, "has": true,
		"how": true, "our": true, "new": true, "one": true, "two": true,
		"more": true, "what": true, "when": true, "where": true, "who": true,
		"why": true, "which": true,
	}
	
	return commonWords[strings.ToLower(word)]
}