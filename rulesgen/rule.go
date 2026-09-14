package rulesgen

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlString reproduces the Python script's yaml_quote scalar policy byte for
// byte: strings containing a backslash emit single-quoted (no further escaping
// needed), everything else double-quoted. Real Go bools stay native so yaml.v3
// emits true/false unquoted — the property the Python emitter exists to
// guarantee because the Go config parser rejects quoted bools.
type yamlString string

func (s yamlString) MarshalYAML() (interface{}, error) {
	n := &yaml.Node{Kind: yaml.ScalarNode, Value: string(s)}
	if strings.Contains(string(s), `\`) {
		n.Style = yaml.SingleQuotedStyle
	} else {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n, nil
}

func emitYAML(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type ruleAction struct {
	Type        yamlString `json:"type" yaml:"type"`
	Destination yamlString `json:"destination,omitempty" yaml:"destination,omitempty"`
}

// watchClient key order matches the script's build_watch_rule_* construction
// order exactly (sender_regex, then list_unsubscribe, then the optionals).
type watchClient struct {
	ListIDRegex       []yamlString `json:"list_id_regex,omitempty" yaml:"list_id_regex,omitempty"`
	SenderRegex       []yamlString `json:"sender_regex,omitempty" yaml:"sender_regex,omitempty"`
	RecipientTagRegex []yamlString `json:"recipient_tag_regex,omitempty" yaml:"recipient_tag_regex,omitempty"`
	ListUnsubscribe   *bool        `json:"list_unsubscribe,omitempty" yaml:"list_unsubscribe,omitempty"`
	ReplyToRegex      []yamlString `json:"replyto_regex,omitempty" yaml:"replyto_regex,omitempty"`
	RecipientsRegex   []yamlString `json:"recipients_regex,omitempty" yaml:"recipients_regex,omitempty"`
}

type ageWindow struct {
	Min yamlString `json:"min,omitempty" yaml:"min,omitempty"`
	Max yamlString `json:"max,omitempty" yaml:"max,omitempty"`
}

// cleanupServer key order matches the script's build_cleanup_rule_* order
// (folders, substring, then the optionals); the Go-only age_window.min
// extension rides last so the script-shared fields stay byte-identical.
type cleanupServer struct {
	Folders          []yamlString `json:"folders,omitempty" yaml:"folders,omitempty"`
	ListIDSubstring  []yamlString `json:"list_id_substring,omitempty" yaml:"list_id_substring,omitempty"`
	SenderSubstring  []yamlString `json:"sender_substring,omitempty" yaml:"sender_substring,omitempty"`
	ListUnsubscribe  *bool        `json:"list_unsubscribe,omitempty" yaml:"list_unsubscribe,omitempty"`
	ReplyToSubstring []yamlString `json:"replyto_substring,omitempty" yaml:"replyto_substring,omitempty"`
	Recipients       []yamlString `json:"recipients,omitempty" yaml:"recipients,omitempty"`
	AgeWindow        *ageWindow   `json:"age_window,omitempty" yaml:"age_window,omitempty"`
}

type Rule struct {
	Name    yamlString     `json:"name" yaml:"name"`
	Client  *watchClient   `json:"client,omitempty" yaml:"client,omitempty"`
	Server  *cleanupServer `json:"server,omitempty" yaml:"server,omitempty"`
	Actions []ruleAction   `json:"actions" yaml:"actions"`
}

func toYamlStrings(values []string) []yamlString {
	out := make([]yamlString, 0, len(values))
	for _, v := range values {
		out = append(out, yamlString(v))
	}
	return out
}

// rulesDoc is the shape of the watch and both cleanup fragment files.
type rulesDoc struct {
	Rules []Rule `yaml:"rules"`
}

// ignoreIdentity is the per-(cluster, lane) Ignored payload. Which field is
// populated follows ADR 0002: list_ids for list_lens, sender_domains for
// sender_unsub_lens, recipient_tags for recipient_tag_lens.
type ignoreIdentity struct {
	ListIDs       []string `json:"list_ids"`
	SenderDomains []string `json:"sender_domains"`
	RecipientTags []string `json:"recipient_tags"`
}

// ignoreSideMatchers key order matches the script's deduped ignore fragment
// output for the canonical report lens order (list, then sender, then tag).
type ignoreSideMatchers struct {
	ListIDs       []yamlString `yaml:"list_ids,omitempty"`
	SenderDomains []yamlString `yaml:"sender_domains,omitempty"`
	RecipientTags []yamlString `yaml:"recipient_tags,omitempty"`
}

type ignoreSides struct {
	Watch   *ignoreSideMatchers `yaml:"watch,omitempty"`
	Cleanup *ignoreSideMatchers `yaml:"cleanup,omitempty"`
}

// ignoreDoc is the shape of the ignore.yaml fragment.
type ignoreDoc struct {
	Ignore ignoreSides `yaml:"ignore"`
}
