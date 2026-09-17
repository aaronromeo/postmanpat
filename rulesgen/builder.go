package rulesgen

import (
	"fmt"
	"strings"
)

// escapeRegex mirrors Python 3.7+ re.escape: backslash in front of exactly
//
//	space # $ & ( ) * + - . ? [ \ ] ^ { | } ~
//
// Note regexp.QuoteMeta would diverge on - & ~ # and whitespace, breaking
// byte parity with the script's escaped defaults (ADR 0003).
func escapeRegex(value string) string {
	const pythonEscape = " #$&()*+-.?[]\\^{|}~"
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if strings.ContainsRune(pythonEscape, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// GenerateInputs is the server-side re-validated rule form. Empty optional
// string fields mean "omit the matcher", matching prompt_optional_list's None
// answer; a nil ListUnsubscribe means "derive from the cluster keys" exactly
// as the script derives it automatically.
type GenerateInputs struct {
	Name              string `json:"name"`
	Action            string `json:"action"`
	Destination       string `json:"destination"`
	Folders           string `json:"folders"`
	AgeWindowMin      string `json:"age_window_min"`
	ListIDRegex       string `json:"list_id_regex"`
	SenderRegex       string `json:"sender_regex"`
	RecipientTagRegex string `json:"recipient_tag_regex"`
	ListIDSubstring   string `json:"list_id_substring"`
	SenderSubstring   string `json:"sender_substring"`
	ListUnsubscribe   *bool  `json:"list_unsubscribe"`
	ReplyToRegex      string `json:"replyto_regex"`
	RecipientsRegex   string `json:"recipients_regex"`
	ReplyToSubstring  string `json:"replyto_substring"`
	Recipients        string `json:"recipients"`
}

func laneForLens(lens string, lane Lane) bool {
	for _, l := range lensLanes[lens] {
		if l == lane {
			return true
		}
	}
	return false
}

func rulesActions(in GenerateInputs) ([]ruleAction, error) {
	switch in.Action {
	case "delete", "":
		return []ruleAction{{Type: yamlString("delete")}}, nil
	case "move":
		if strings.TrimSpace(in.Destination) == "" {
			return nil, fmt.Errorf("move action requires a destination")
		}
		return []ruleAction{{Type: yamlString("move"), Destination: yamlString(in.Destination)}}, nil
	default:
		return nil, fmt.Errorf("invalid action %q", in.Action)
	}
}

func keyStrings(keys map[string]any, field string) []string {
	raw, ok := keys[field]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case []string:
		return v
	}
	return nil
}

// mergedSenderDomains mirrors the script's
//
//	merged = list(dict.fromkeys([*sender_domains, *from_domains]))
//
// where from_domains are the parts after "@" of each FromList value that has
// exactly one "@".
func mergedSenderDomains(keys map[string]any) []string {
	var merged []string
	appendUnique := func(value string) {
		if value == "" {
			return
		}
		for _, existing := range merged {
			if existing == value {
				return
			}
		}
		merged = append(merged, value)
	}
	for _, domain := range keyStrings(keys, "SenderDomains") {
		appendUnique(domain)
	}
	for _, value := range keyStrings(keys, "FromList") {
		parts := strings.Split(value, "@")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			appendUnique(strings.TrimSpace(parts[1]))
		}
	}
	return merged
}

// listUnsubscribeFromKeys returns a non-nil bool only when the cluster keys
// carry HasListUnsubscribe as a JSON bool, matching the script's
// isinstance(has_unsub, bool) guard.
func listUnsubscribeFromKeys(keys map[string]any) *bool {
	raw, ok := keys["HasListUnsubscribe"]
	if !ok {
		return nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil
	}
	return &b
}

func listUnsubscribeFor(in GenerateInputs, keys map[string]any) *bool {
	if in.ListUnsubscribe != nil {
		return in.ListUnsubscribe
	}
	return listUnsubscribeFromKeys(keys)
}

func optionalValues(value string) []yamlString {
	if value == "" {
		return nil
	}
	return toYamlStrings(splitList(value))
}

func watchListRule(c Cluster, in GenerateInputs) (Rule, error) {
	if in.Name == "" {
		return Rule{}, fmt.Errorf("rule name is required")
	}
	regex := splitList(in.ListIDRegex)
	if len(regex) <= 0 {
		return Rule{}, fmt.Errorf("list_id_regex is required")
	}
	actions, err := rulesActions(in)
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		Name:    yamlString(in.Name),
		Client:  &watchClient{ListIDRegex: toYamlStrings(regex)},
		Actions: actions,
	}, nil
}

func watchSenderRule(c Cluster, in GenerateInputs) (Rule, error) {
	if in.Name == "" {
		return Rule{}, fmt.Errorf("rule name is required")
	}
	sender := in.SenderRegex
	if sender == "" {
		sender = strings.Join(escapedDomains(mergedSenderDomains(c.Keys)), ", ")
	}
	values := splitList(sender)
	if len(values) <= 0 {
		return Rule{}, fmt.Errorf("sender_regex is required")
	}
	actions, err := rulesActions(in)
	if err != nil {
		return Rule{}, err
	}
	client := &watchClient{SenderRegex: toYamlStrings(values)}
	if lu := listUnsubscribeFor(in, c.Keys); lu != nil {
		client.ListUnsubscribe = lu
	}
	client.ReplyToRegex = optionalValues(in.ReplyToRegex)
	client.RecipientsRegex = optionalValues(in.RecipientsRegex)
	return Rule{Name: yamlString(in.Name), Client: client, Actions: actions}, nil
}

func watchTagRule(c Cluster, in GenerateInputs) (Rule, error) {
	if in.Name == "" {
		return Rule{}, fmt.Errorf("rule name is required")
	}
	regex := splitList(in.RecipientTagRegex)
	if len(regex) <= 0 {
		return Rule{}, fmt.Errorf("recipient_tag_regex is required")
	}
	actions, err := rulesActions(in)
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		Name:    yamlString(in.Name),
		Client:  &watchClient{RecipientTagRegex: toYamlStrings(regex)},
		Actions: actions,
	}, nil
}

func cleanupListRule(c Cluster, in GenerateInputs) (Rule, error) {
	if in.Name == "" {
		return Rule{}, fmt.Errorf("rule name is required")
	}
	substring := splitList(in.ListIDSubstring)
	if len(substring) <= 0 {
		return Rule{}, fmt.Errorf("list_id_substring is required")
	}
	folders := splitList(in.Folders)
	if len(folders) <= 0 {
		return Rule{}, fmt.Errorf("folders are required")
	}
	actions, err := rulesActions(in)
	if err != nil {
		return Rule{}, err
	}
	server := &cleanupServer{
		Folders:         toYamlStrings(folders),
		ListIDSubstring: toYamlStrings(substring),
	}
	server.AgeWindow = ageWindowFor(in.AgeWindowMin)
	return Rule{Name: yamlString(in.Name), Server: server, Actions: actions}, nil
}

func cleanupSenderRules(c Cluster, in GenerateInputs) ([]Rule, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("rule name is required")
	}
	substring := splitList(in.SenderSubstring)
	if len(substring) <= 0 {
		return nil, fmt.Errorf("sender_substring is required")
	}
	folders := splitList(in.Folders)
	if len(folders) <= 0 {
		return nil, fmt.Errorf("folders are required")
	}
	actions, err := rulesActions(in)
	if err != nil {
		return nil, err
	}
	base := &cleanupServer{
		Folders:         toYamlStrings(folders),
		SenderSubstring: toYamlStrings(substring),
	}
	if lu := listUnsubscribeFor(in, c.Keys); lu != nil {
		base.ListUnsubscribe = lu
	}
	base.ReplyToSubstring = optionalValues(in.ReplyToSubstring)
	base.AgeWindow = ageWindowFor(in.AgeWindowMin)

	recipients := splitList(in.Recipients)
	if len(recipients) == 0 {
		copy := *base
		return []Rule{{Name: yamlString(in.Name), Server: &copy, Actions: actions}}, nil
	}
	if len(recipients) == 1 {
		copy := *base
		copy.Recipients = toYamlStrings(recipients)
		return []Rule{{Name: yamlString(in.Name), Server: &copy, Actions: actions}}, nil
	}
	out := make([]Rule, 0, len(recipients))
	for _, recipient := range recipients {
		copy := *base
		copy.Recipients = []yamlString{yamlString(recipient)}
		out = append(out, Rule{
			Name:    yamlString(in.Name + " (" + recipient + ")"),
			Server:  &copy,
			Actions: actions,
		})
	}
	return out, nil
}

func ageWindowFor(min string) *ageWindow {
	if min == "" {
		return nil
	}
	return &ageWindow{Min: yamlString(min)}
}

// BuildRules is the parity-critical core: cluster + lane + validated form
// inputs -> fully-formed rules ready for YAML emission. Cleanup sender rules
// with several recipients split one-rule-per-alias exactly as the script does
// (server matchers AND multiple recipients, so a multi-alias rule can never
// match).
func BuildRules(c Cluster, lane Lane, in GenerateInputs) ([]Rule, error) {
	if !laneForLens(c.Lens, lane) {
		return nil, fmt.Errorf("lane %q is not offered for lens %q", lane, c.Lens)
	}
	switch {
	case lane == LaneWatch && c.Lens == "list_lens":
		rule, err := watchListRule(c, in)
		if err != nil {
			return nil, err
		}
		return []Rule{rule}, nil
	case lane == LaneWatch && c.Lens == "sender_unsub_lens":
		rule, err := watchSenderRule(c, in)
		if err != nil {
			return nil, err
		}
		return []Rule{rule}, nil
	case lane == LaneWatch && c.Lens == "recipient_tag_lens":
		rule, err := watchTagRule(c, in)
		if err != nil {
			return nil, err
		}
		return []Rule{rule}, nil
	case (lane == LaneOneTimeCleanup || lane == LaneOngoingCleanup) && c.Lens == "list_lens":
		rule, err := cleanupListRule(c, in)
		if err != nil {
			return nil, err
		}
		return []Rule{rule}, nil
	case (lane == LaneOneTimeCleanup || lane == LaneOngoingCleanup) && c.Lens == "sender_unsub_lens":
		return cleanupSenderRules(c, in)
	default:
		return nil, fmt.Errorf("unsupported lens %q for lane %q", c.Lens, lane)
	}
}

func escapedDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		out = append(out, escapeRegex(d))
	}
	return out
}

// IgnoreIdentity extracts the ADR 0002 ignore identity for a cluster's lens:
// list_ids for list_lens, sender_domains for sender_unsub_lens, recipient_tags
// for recipient_tag_lens. Mirrors extract_ignore_identity.
func IgnoreIdentity(c Cluster) ignoreIdentity {
	switch c.Lens {
	case "list_lens":
		if raw, ok := c.Keys["ListID"]; ok {
			if value := fmt.Sprint(raw); value != "" {
				return ignoreIdentity{ListIDs: []string{value}}
			}
		}
	case "sender_unsub_lens":
		if domains := keyStrings(c.Keys, "SenderDomains"); len(domains) > 0 {
			return ignoreIdentity{SenderDomains: domains}
		}
	case "recipient_tag_lens":
		if raw, ok := c.Keys["recipient_tag"]; ok {
			if value := fmt.Sprint(raw); value != "" {
				return ignoreIdentity{RecipientTags: []string{value}}
			}
		}
	}
	return ignoreIdentity{}
}
