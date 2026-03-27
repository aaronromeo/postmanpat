// Package analysis provides shared analysis types and functions
package analysis

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
)

// Options contains options for running analysis
type Options struct {
	Top      int
	Examples int
	MinCount int
}

// DefaultOptions returns default analysis options
func DefaultOptions() Options {
	return Options{
		Top:      100,
		Examples: 20,
		MinCount: 2,
	}
}

// Lens represents a single lens with its clusters
type Lens struct {
	KeyFields []string  `json:"key_fields"`
	Clusters  []Cluster `json:"clusters"`
}

// Cluster represents a single cluster of similar emails
type Cluster struct {
	ClusterID  string          `json:"cluster_id"`
	Count      int             `json:"count"`
	LatestDate time.Time       `json:"latest_date"`
	Keys       map[string]any  `json:"keys"`
	Signals    ClusterSignals  `json:"signals"`
	Examples   ClusterExamples `json:"examples"`
}

// ClusterSignals contains signal information about a cluster
type ClusterSignals struct {
	HasListID            bool           `json:"has_list_id"`
	HasListUnsubscribe   bool           `json:"has_list_unsubscribe"`
	PrecedenceCategories map[string]int `json:"precedence_categories"`
}

// ClusterExamples contains example data from a cluster
type ClusterExamples struct {
	SubjectRaw             []string `json:"subject_raw"`
	Recipients             []string `json:"recipients"`
	ReplyToDomains         []string `json:"reply_to_domains"`
	SenderDomains          []string `json:"sender_domains"`
	ReturnPathDomains      []string `json:"returnpath_domains"`
	ListUnsubscribeTargets []string `json:"list_unsubscribe_targets"`
}

// clusterAccumulator holds intermediate cluster data during analysis
type clusterAccumulator struct {
	count          int
	keys           map[string]any
	hasListID      bool
	hasUnsubscribe bool
	precedence     map[string]int
	latestDate     time.Time
	examples       ClusterExamples
	exampleSets    map[string]map[string]struct{}
}

type ReportParams struct {
	Mailbox   string
	Account   string
	Generated time.Time
	AgeWindow *appconfig.AgeWindow
	Options   Options
}

type Source struct {
	Mailbox    string     `json:"mailbox"`
	Account    string     `json:"account"`
	TimeWindow TimeWindow `json:"time_window"`
}

type Stats struct {
	TotalMessagesScanned int `json:"total_messages_scanned"`
}

type Indexes struct {
	ListLens         Lens `json:"list_lens"`
	SenderLens       Lens `json:"sender_unsub_lens"`
	TemplateLens     Lens `json:"template_lens"`
	RecipientTagLens Lens `json:"recipient_tag_lens"`
}

type Report struct {
	GeneratedAt string  `json:"generated_at"`
	Source      Source  `json:"source"`
	Stats       Stats   `json:"stats"`
	Indexes     Indexes `json:"indexes"`
}

type TimeWindow struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

const (
	exampleKeySubjectRaw             = "subject_raw"
	exampleKeyRecipients             = "recipients"
	exampleKeyReplyToDomains         = "reply_to_domains"
	exampleKeySenderDomains          = "sender_domains"
	exampleKeyReturnPathDomains      = "returnpath_domains"
	exampleKeyListUnsubscribeTargets = "list_unsubscribe_targets"
)

// BuildListLens builds the list lens clusters from mail data
func BuildListLens(data []imap.MailData, options Options) Lens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		listID := normalizeListID(item.ListID)
		if listID == "" {
			continue
		}
		keyString := fmt.Sprintf("ListID=%s", listID)
		clusterID := makeClusterID("list_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"ListID": listID,
		})
		accumulateCluster(acc, item, true, options.Examples)
	}

	return Lens{
		KeyFields: []string{"ListID"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

// BuildSenderUnsubLens builds the sender unsubscribe lens clusters from mail data
func BuildSenderUnsubLens(data []imap.MailData, options Options) Lens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		senderDomains := normalizeDomains(item.SenderDomains)
		if len(senderDomains) == 1 && senderDomains[0] == "" {
			continue
		}
		hasUnsub := item.ListUnsubscribe
		keyString := fmt.Sprintf("SenderDomains=%s|HasListUnsubscribe=%v", strings.Join(senderDomains, ","), hasUnsub)
		clusterID := makeClusterID("sender_unsub_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"SenderDomains":      senderDomains,
			"HasListUnsubscribe": hasUnsub,
			"FromList":           item.From,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples)
	}

	return Lens{
		KeyFields: []string{"SenderDomains", "HasListUnsubscribe"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

// BuildTemplateLens builds the template lens clusters from mail data
func BuildTemplateLens(data []imap.MailData, options Options) Lens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		senderDomains := normalizeDomains(item.SenderDomains)
		subject := item.SubjectNormalized
		keyString := fmt.Sprintf("SenderDomains=%s|SubjectNormalized=%s", strings.Join(senderDomains, ","), subject)
		clusterID := makeClusterID("template_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"SenderDomains":     senderDomains,
			"SubjectNormalized": subject,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples)
	}

	return Lens{
		KeyFields: []string{"SenderDomains", "SubjectNormalized"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

// BuildRecipientTagLens builds the recipient tag lens clusters from mail data
func BuildRecipientTagLens(data []imap.MailData, options Options) Lens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		tags := normalizeRecipientTags(item.RecipientTags)
		if len(tags) == 0 {
			continue
		}
		joined := strings.Join(tags, ",")
		keyString := fmt.Sprintf("recipient_tag=%s", joined)
		clusterID := makeClusterID("recipient_tag_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"recipient_tag": joined,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples)
	}

	return Lens{
		KeyFields: []string{"recipient_tag"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

// Helper functions

func normalizeListID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeDomains(domains []string) []string {
	seen := make(map[string]struct{})
	for _, part := range domains {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized == "" {
			continue
		}
		seen[normalized] = struct{}{}
	}
	if len(seen) == 0 {
		return []string{""}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeRecipientTags(tags []string) []string {
	seen := make(map[string]struct{})
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" {
			continue
		}
		seen[normalized] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func makeClusterID(lens, keyString string) string {
	hash := sha1.Sum([]byte(keyString))
	return fmt.Sprintf("%s:%s", lens, hex.EncodeToString(hash[:]))
}

func ensureClusterAccumulator(clusters map[string]*clusterAccumulator, clusterID string, keys map[string]any) *clusterAccumulator {
	acc, ok := clusters[clusterID]
	if ok {
		return acc
	}
	acc = &clusterAccumulator{
		count:          0,
		keys:           keys,
		hasListID:      true,
		hasUnsubscribe: true,
		precedence:     make(map[string]int),
		exampleSets: map[string]map[string]struct{}{
			exampleKeySubjectRaw:             {},
			exampleKeyRecipients:             {},
			exampleKeyReplyToDomains:         {},
			exampleKeySenderDomains:          {},
			exampleKeyReturnPathDomains:      {},
			exampleKeyListUnsubscribeTargets: {},
		},
	}
	clusters[clusterID] = acc
	return acc
}

func accumulateCluster(acc *clusterAccumulator, item imap.MailData, hasListID bool, maxExamples int) {
	acc.count++
	if !hasListID {
		acc.hasListID = false
	}
	if !item.ListUnsubscribe {
		acc.hasUnsubscribe = false
	}
	if !item.MessageDate.IsZero() && (acc.latestDate.IsZero() || item.MessageDate.After(acc.latestDate)) {
		acc.latestDate = item.MessageDate
	}

	precedence := normalizePrecedenceCategory(item.PrecedenceCategory)
	acc.precedence[precedence]++

	addExample(acc, exampleKeySubjectRaw, strings.TrimSpace(item.SubjectRaw), maxExamples)
	for _, recipient := range item.Recipients {
		addExample(acc, exampleKeyRecipients, recipient, maxExamples)
	}
	for _, replyTo := range item.ReplyToDomains {
		addExample(acc, exampleKeyReplyToDomains, replyTo, maxExamples)
	}
	for _, senderDomain := range item.SenderDomains {
		addExample(acc, exampleKeySenderDomains, senderDomain, maxExamples)
	}
	if strings.TrimSpace(item.ReturnPathDomain) != "" {
		addExample(acc, exampleKeyReturnPathDomains, item.ReturnPathDomain, maxExamples)
	}
	for _, target := range splitAndTrim(item.ListUnsubscribeTargets) {
		addExample(acc, exampleKeyListUnsubscribeTargets, target, maxExamples)
	}
}

func normalizePrecedenceCategory(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "bulk", "list", "junk", "first-class":
		return normalized
	default:
		return "unknown"
	}
}

func splitAndTrim(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func addExample(acc *clusterAccumulator, key, value string, maxExamples int) {
	if value == "" || maxExamples <= 0 {
		return
	}
	seen := acc.exampleSets[key]
	if _, ok := seen[value]; ok {
		return
	}
	if len(seen) >= maxExamples {
		return
	}
	seen[value] = struct{}{}

	switch key {
	case exampleKeySubjectRaw:
		acc.examples.SubjectRaw = append(acc.examples.SubjectRaw, value)
	case exampleKeyRecipients:
		acc.examples.Recipients = append(acc.examples.Recipients, value)
	case exampleKeySenderDomains:
		acc.examples.SenderDomains = append(acc.examples.SenderDomains, value)
	case exampleKeyReplyToDomains:
		acc.examples.ReplyToDomains = append(acc.examples.ReplyToDomains, value)
	case exampleKeyListUnsubscribeTargets:
		acc.examples.ListUnsubscribeTargets = append(acc.examples.ListUnsubscribeTargets, value)
	}
}

func finalizeClusters(clusters map[string]*clusterAccumulator, options Options) []Cluster {
	minCount := options.MinCount
	if minCount <= 0 {
		minCount = 1
	}

	var result []Cluster
	for clusterID, acc := range clusters {
		if acc.count < minCount {
			continue
		}
		result = append(result, Cluster{
			ClusterID:  clusterID,
			Count:      acc.count,
			LatestDate: acc.latestDate,
			Keys:       acc.keys,
			Signals: ClusterSignals{
				HasListID:            acc.hasListID,
				HasListUnsubscribe:   acc.hasUnsubscribe,
				PrecedenceCategories: acc.precedence,
			},
			Examples: acc.examples,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].ClusterID < result[j].ClusterID
	})

	if options.Top > 0 && len(result) > options.Top {
		return result[:options.Top]
	}
	return result
}
