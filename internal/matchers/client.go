package matchers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/config"
)

type ClientMessage struct {
	ListID           string
	SenderDomains    []string
	ReplyToDomains   []string
	SubjectRaw       string
	Recipients       []string
	RecipientTags    []string
	Body             string
	Cc               []string
	ReturnPathDomain string
	ListUnsubscribe  bool
}

// MatchesClient returns true if the message satisfies all configured client matchers.
func MatchesClient(matchers *config.ClientMatchers, data ClientMessage) (bool, error) {
	if matchers == nil || matchers.IsEmpty() {
		return true, nil
	}
	if len(matchers.ListIDRegex) > 0 {
		ok, err := matchAnyRegex(matchers.ListIDRegex, data.ListID)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.SubjectRegex) > 0 {
		ok, err := matchAnyRegex(matchers.SubjectRegex, data.SubjectRaw)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.RecipientsRegex) > 0 {
		ok, err := matchAnyRegexInList(matchers.RecipientsRegex, data.Recipients)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.CcRegex) > 0 {
		ok, err := matchAnyRegexInList(matchers.CcRegex, data.Cc)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.ReturnPathRegex) > 0 {
		ok, err := matchAnyRegex(matchers.ReturnPathRegex, data.ReturnPathDomain)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.SenderRegex) > 0 {
		ok, err := matchAnyRegexInList(matchers.SenderRegex, data.SenderDomains)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.ReplyToRegex) > 0 {
		ok, err := matchAnyRegexInList(matchers.ReplyToRegex, data.ReplyToDomains)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.BodyRegex) > 0 {
		ok, err := matchAnyRegex(matchers.BodyRegex, data.Body)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if len(matchers.RecipientTagRegex) > 0 {
		ok, err := matchAnyRegexInList(matchers.RecipientTagRegex, data.RecipientTags)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if matchers.ListUnsubscribe != nil {
		if data.ListUnsubscribe != *matchers.ListUnsubscribe {
			return false, nil
		}
	}
	return true, nil
}

func matchAnyRegex(patterns []string, value string) (bool, error) {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", pattern, err)
		}
		if re.MatchString(value) {
			return true, nil
		}
	}
	return false, nil
}

func matchAnyRegexInList(patterns []string, values []string) (bool, error) {
	if len(values) == 0 {
		return false, nil
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ok, err := matchAnyRegex(patterns, value)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
