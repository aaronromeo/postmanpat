package cli

import (
	"strings"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
)

// matchesIgnoreMatchers reports whether item matches any entry of m.
// Sender domains match exactly (case-insensitive); all other entry types
// match as case-insensitive substrings.
func matchesIgnoreMatchers(item imap.MailData, m appconfig.IgnoreMatchers) bool {
	return matchesIgnoreDomains(item.SenderDomains, m.SenderDomains) ||
		matchesIgnoreAddresses(item.From, m.SenderAddresses) ||
		matchesIgnoreString(item.ListID, m.ListIDs) ||
		matchesIgnoreTags(item.RecipientTags, m.RecipientTags) ||
		matchesIgnoreString(item.SubjectRaw, m.SubjectSubstrings)
}

func matchesIgnoreDomains(domains, entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		for _, domain := range domains {
			if strings.ToLower(strings.TrimSpace(domain)) == entry {
				return true
			}
		}
	}
	return false
}

func matchesIgnoreAddresses(addresses, entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		for _, addr := range addresses {
			if strings.Contains(strings.ToLower(addr), entry) {
				return true
			}
		}
	}
	return false
}

func matchesIgnoreTags(tags, entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		for _, tag := range tags {
			if strings.Contains(strings.ToLower(tag), entry) {
				return true
			}
		}
	}
	return false
}

func matchesIgnoreString(value string, entries []string) bool {
	if value == "" {
		return false
	}
	value = strings.ToLower(value)
	for _, entry := range entries {
		if strings.Contains(value, strings.ToLower(strings.TrimSpace(entry))) {
			return true
		}
	}
	return false
}

// filterFullyDecided removes messages that match BOTH the Watch Ignore List
// and the Cleanup Ignore List. A nil ignore config returns data unchanged.
func filterFullyDecided(data []imap.MailData, ignore *appconfig.IgnoreConfig) []imap.MailData {
	if ignore == nil {
		return data
	}
	filtered := make([]imap.MailData, 0, len(data))
	for _, item := range data {
		if matchesIgnoreMatchers(item, ignore.Watch) && matchesIgnoreMatchers(item, ignore.Cleanup) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
