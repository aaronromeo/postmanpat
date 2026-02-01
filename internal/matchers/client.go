package matchers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/config"
)

type ClientMessage struct {
	ListID string
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
