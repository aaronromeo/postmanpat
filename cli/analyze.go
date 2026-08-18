package cli

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
	"github.com/spf13/cobra"
)

var analyzeTLSConfigProvider func() *tls.Config

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze IMAP folders and report unique sender domains",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgPath, err := resolveConfigPath(cmd)
		if err != nil {
			return err
		}

		if err := loadEnvFile(); err != nil {
			return err
		}

		cfg, err := appconfig.Load(cfgPath)
		if err != nil {
			return err
		}

		if err := appconfig.Validate(cfg); err != nil {
			return err
		}

		for _, rule := range cfg.Rules {
			if rule.Client != nil {
				return fmt.Errorf("rule %q defines client matchers, which are not supported by analyze", rule.Name)
			}
			if rule.Server == nil {
				return fmt.Errorf("rule %q must define server matchers for analyze", rule.Name)
			}
		}

		outDir, err := cmd.Flags().GetString("out")
		if err != nil {
			return err
		}
		if outDir != "" {
			seen := make(map[string]string)
			for _, rule := range cfg.Rules {
				slug := slugifyRuleName(rule.Name)
				if prev, ok := seen[slug]; ok {
					return fmt.Errorf("analyze --out: rules %q and %q produce the same filename slug %q; rename one", prev, rule.Name, slug)
				}
				seen[slug] = rule.Name
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("analyze --out: %w", err)
			}
		}

		imapEnv, err := appconfig.IMAPEnvFromEnv()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		clientOpts := []imap.Option{
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
		}
		if analyzeTLSConfigProvider != nil {
			clientOpts = append(clientOpts, imap.WithTLSConfig(analyzeTLSConfigProvider()))
		}
		client := serverrunner.New(clientOpts...)
		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		topN, err := cmd.Flags().GetInt("top")
		if err != nil {
			return err
		}
		examplesN, err := cmd.Flags().GetInt("examples")
		if err != nil {
			return err
		}
		minCount, err := cmd.Flags().GetInt("min-count")
		if err != nil {
			return err
		}
		noIgnore, err := cmd.Flags().GetBool("no-ignore")
		if err != nil {
			return err
		}
		ignore := cfg.Ignore
		if noIgnore {
			ignore = nil
		}
		options := analyzeOptions{
			Top:      topN,
			Examples: examplesN,
			MinCount: minCount,
		}

		for _, rule := range cfg.Rules {
			if rule.Client != nil {
				return fmt.Errorf("rule %q defines client matchers, which are not supported by analyze", rule.Name)
			}
			if rule.Server == nil {
				return fmt.Errorf("rule %q must define server matchers for analyze", rule.Name)
			}
			mailbox := rule.Server.Folders[0]

			matched, err := client.SearchByServerMatchers(ctx, *rule.Server)
			if err != nil {
				_ = client.Close()
				return err
			}

			dataByMailbox, err := client.FetchSenderDataByMailbox(ctx, matched)
			if err != nil {
				return err
			}

			data := dataByMailbox[mailbox]
			if !noIgnore {
				data = filterFullyDecided(data, cfg.Ignore)
			}
			report, err := buildAnalyzeReport(data, analyzeReportParams{
				Mailbox:   mailbox,
				Account:   imapEnv.User,
				Generated: time.Now().UTC(),
				AgeWindow: rule.Server.AgeWindow,
				Options:   options,
				Ignore:    ignore,
			})
			if err != nil {
				return err
			}

			path, err := writeAnalyzeReport(report)
			if outDir != "" {
				path, err = writeAnalyzeReportToDir(report, outDir, slugifyRuleName(rule.Name))
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
		}

		return nil
	},
}

func init() {
	analyzeCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_CONFIG)")
	analyzeCmd.Flags().Bool("verbose", false, "Enable verbose logging")
	analyzeCmd.Flags().Int("top", 100, "Maximum clusters per lens")
	analyzeCmd.Flags().Int("examples", 20, "Maximum examples per field")
	analyzeCmd.Flags().Int("min-count", 2, "Minimum cluster count to include")
	analyzeCmd.Flags().Bool("no-ignore", false, "Disable ignore-list filtering")
	analyzeCmd.Flags().String("out", "", "Directory to write reports into with deterministic per-rule filenames (overwritten each run). If unset, writes to a temp file and prints its path.")
}

type analyzeReportParams struct {
	Mailbox   string
	Account   string
	Generated time.Time
	AgeWindow *appconfig.AgeWindow
	Options   analyzeOptions
	Ignore    *appconfig.IgnoreConfig
}

type analyzeOptions struct {
	Top      int
	Examples int
	MinCount int
}

type analyzeTimeWindow struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

type analyzeSource struct {
	Mailbox    string            `json:"mailbox"`
	Account    string            `json:"account"`
	TimeWindow analyzeTimeWindow `json:"time_window"`
}

type analyzeStats struct {
	TotalMessagesScanned int `json:"total_messages_scanned"`
}

// type analyzeRawRecord struct {
// 	SenderDomains          []string `json:"SenderDomains"`
// 	ReplyToDomains         []string `json:"ReplyToDomains"`
// 	Recipients             []string `json:"Recipients"`
// 	RecipientTags          []string `json:"RecipientTags"`
// 	ListID                 string   `json:"ListID"`
// 	ListUnsubscribe        bool     `json:"ListUnsubscribe"`
// 	ListUnsubscribeTargets string   `json:"ListUnsubscribeTargets"`
// 	PrecedenceRaw          string   `json:"PrecedenceRaw"`
// 	PrecedenceCategory     string   `json:"PrecedenceCategory"`
// 	XMailer                string   `json:"XMailer"`
// 	UserAgent              string   `json:"UserAgent"`
// 	SubjectRaw             string   `json:"SubjectRaw"`
// 	SubjectNormalized      string   `json:"SubjectNormalized"`
// }

type analyzeIndexes struct {
	// Raw              []analyzeRawRecord `json:"raw"`
	ListLens         analyzeLens `json:"list_lens"`
	SenderLens       analyzeLens `json:"sender_unsub_lens"`
	TemplateLens     analyzeLens `json:"template_lens"`
	RecipientTagLens analyzeLens `json:"recipient_tag_lens"`
}

type analyzeLens struct {
	KeyFields []string         `json:"key_fields"`
	Clusters  []analyzeCluster `json:"clusters"`
}

type analyzeCluster struct {
	ClusterID  string                 `json:"cluster_id"`
	Count      int                    `json:"count"`
	LatestDate time.Time              `json:"latest_date"`
	Keys       map[string]any         `json:"keys"`
	Signals    analyzeClusterSignals  `json:"signals"`
	Examples   analyzeClusterExamples `json:"examples"`
	Suppressed []string               `json:"suppressed,omitempty"`
}

type analyzeClusterSignals struct {
	HasListID            bool           `json:"has_list_id"`
	HasListUnsubscribe   bool           `json:"has_list_unsubscribe"`
	PrecedenceCategories map[string]int `json:"precedence_categories"`
}

type analyzeClusterExamples struct {
	SubjectRaw             []string `json:"subject_raw"`
	Recipients             []string `json:"recipients"`
	ReplyToDomains         []string `json:"reply_to_domains"`
	SenderDomains          []string `json:"sender_domains"`
	ReturnPathDomains      []string `json:"returnpath_domains"`
	ListUnsubscribeTargets []string `json:"list_unsubscribe_targets"`
}

type analyzeReport struct {
	GeneratedAt string         `json:"generated_at"`
	Source      analyzeSource  `json:"source"`
	Stats       analyzeStats   `json:"stats"`
	Indexes     analyzeIndexes `json:"indexes"`
}

type timeWindow struct {
	After  string
	Before string
}

type clusterAccumulator struct {
	count           int
	keys            map[string]any
	hasListID       bool
	hasUnsubscribe  bool
	precedence      map[string]int
	latestDate      time.Time
	examples        analyzeClusterExamples
	exampleSets     map[string]map[string]struct{}
	suppressWatch   bool
	suppressCleanup bool
}

const (
	ExampleKeySubjectRaw             = "subject_raw"
	ExampleKeyRecipients             = "recipients"
	ExampleKeyReplyToDomains         = "reply_to_domains"
	ExampleKeySenderDomains          = "sender_domains"
	ExampleKeyReturnPathDomains      = "returnpath_domains"
	ExampleKeyListUnsubscribeTargets = "list_unsubscribe_targets"
)

func buildTimeWindow(now time.Time, window *appconfig.AgeWindow) (timeWindow, error) {
	after, before, err := appconfig.AgeWindowBounds(now, window)
	if err != nil {
		return timeWindow{}, err
	}
	if before == "" {
		before = now.Format(time.RFC3339)
	}
	return timeWindow{After: after, Before: before}, nil
}

func buildAnalyzeReport(data []imap.MailData, params analyzeReportParams) (analyzeReport, error) {
	window, err := buildTimeWindow(params.Generated, params.AgeWindow)
	if err != nil {
		return analyzeReport{}, err
	}
	// raw := make([]analyzeRawRecord, 0, len(data))
	// for _, item := range data {
	// 	raw = append(raw, analyzeRawRecord{
	// 		SenderDomains:          item.SenderDomains,
	// 		ReplyToDomains:         item.ReplyToDomains,
	// 		Recipients:             item.Recipients,
	// 		RecipientTags:          item.RecipientTags,
	// 		ListID:                 item.ListID,
	// 		ListUnsubscribe:        item.ListUnsubscribe,
	// 		ListUnsubscribeTargets: item.ListUnsubscribeTargets,
	// 		PrecedenceRaw:          item.PrecedenceRaw,
	// 		PrecedenceCategory:     item.PrecedenceCategory,
	// 		XMailer:                item.XMailer,
	// 		UserAgent:              item.UserAgent,
	// 		SubjectRaw:             item.SubjectRaw,
	// 		SubjectNormalized:      item.SubjectNormalized,
	// 	})
	// }

	options := params.Options
	listLens := buildListLens(data, options, params.Ignore)
	senderLens := buildSenderUnsubLens(data, options, params.Ignore)
	templateLens := buildTemplateLens(data, options, params.Ignore)
	recipientTagLens := buildRecipientTagLens(data, options, params.Ignore)

	return analyzeReport{
		GeneratedAt: params.Generated.Format(time.RFC3339),
		Source: analyzeSource{
			Mailbox: params.Mailbox,
			Account: params.Account,
			TimeWindow: analyzeTimeWindow{
				After:  window.After,
				Before: window.Before,
			},
		},
		Stats: analyzeStats{
			TotalMessagesScanned: len(data),
		},
		Indexes: analyzeIndexes{
			// Raw:              raw,
			ListLens:         listLens,
			SenderLens:       senderLens,
			TemplateLens:     templateLens,
			RecipientTagLens: recipientTagLens,
		},
	}, nil
}

func writeAnalyzeReport(report analyzeReport) (string, error) {
	tmpFile, err := os.CreateTemp("", "postmanpat-analyze-*.json")
	if err != nil {
		return "", err
	}
	path := tmpFile.Name()
	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(report); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// slugifyRuleName turns a rule name into a stable, filesystem-safe slug used to
// build deterministic report filenames. Runs of non-alphanumeric characters
// collapse to a single dash; leading/trailing dashes are trimmed.
func slugifyRuleName(name string) string {
	s := strings.ToLower(name)
	var b strings.Builder
	inRun := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			inRun = false
			continue
		}
		if !inRun {
			b.WriteRune('-')
			inRun = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// writeAnalyzeReportToDir writes a report to a deterministic path inside dir,
// named postmanpat-analyze-<slug>.json. The write is atomic: bytes land in a
// temp file in the same directory, are synced, then renamed over the target so
// a reader never observes a half-written report. The previous file, if any, is
// replaced.
func writeAnalyzeReportToDir(report analyzeReport, dir, slug string) (string, error) {
	target := filepath.Join(dir, fmt.Sprintf("postmanpat-analyze-%s.json", slug))
	tmpFile, err := os.CreateTemp(dir, ".postmanpat-analyze-*.json.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	encErr := encoder.Encode(report)
	syncErr := tmpFile.Sync()
	closeErr := tmpFile.Close()
	if encErr != nil {
		_ = os.Remove(tmpPath)
		return "", encErr
	}
	if syncErr != nil {
		_ = os.Remove(tmpPath)
		return "", syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return target, nil
}

func buildListLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
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
		accumulateCluster(acc, item, true, options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"ListID"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

func buildSenderUnsubLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		senderDomains := normalizeDomains(item.SenderDomains)
		if len(senderDomains) == 1 && strings.TrimSpace(senderDomains[0]) == "" {
			continue
		}
		hasUnsub := item.ListUnsubscribe
		keyString := fmt.Sprintf("SenderDomains=%s|HasListUnsubscribe=%s", strings.Join(senderDomains, ","), boolString(hasUnsub))
		clusterID := makeClusterID("sender_unsub_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"SenderDomains":      senderDomains,
			"HasListUnsubscribe": hasUnsub,
			"FromList":           item.From,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"SenderDomains", "HasListUnsubscribe"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

func buildTemplateLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
	clusters := make(map[string]*clusterAccumulator)
	for _, item := range data {
		senderDomains := normalizeDomains(item.SenderDomains)
		subject := strings.TrimSpace(item.SubjectNormalized)
		keyString := fmt.Sprintf("SenderDomains=%s|SubjectNormalized=%s", strings.Join(senderDomains, ","), subject)
		clusterID := makeClusterID("template_lens", keyString)
		acc := ensureClusterAccumulator(clusters, clusterID, map[string]any{
			"SenderDomains":     senderDomains,
			"SubjectNormalized": subject,
		})
		accumulateCluster(acc, item, item.ListID != "", options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"SenderDomains", "SubjectNormalized"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

func buildRecipientTagLens(data []imap.MailData, options analyzeOptions, ignore *appconfig.IgnoreConfig) analyzeLens {
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
		accumulateCluster(acc, item, item.ListID != "", options.Examples, ignore)
	}

	return analyzeLens{
		KeyFields: []string{"recipient_tag"},
		Clusters:  finalizeClusters(clusters, options),
	}
}

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
	normalized := make([]string, 0, len(seen))
	for value := range seen {
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
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
	normalized := make([]string, 0, len(seen))
	for value := range seen {
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
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
		examples: analyzeClusterExamples{
			SubjectRaw:             []string{},
			Recipients:             []string{},
			ReplyToDomains:         []string{},
			ListUnsubscribeTargets: []string{},
		},
		exampleSets: map[string]map[string]struct{}{
			ExampleKeySubjectRaw:             {},
			ExampleKeyRecipients:             {},
			ExampleKeyReplyToDomains:         {},
			ExampleKeySenderDomains:          {},
			ExampleKeyReturnPathDomains:      {},
			ExampleKeyListUnsubscribeTargets: {},
		},
	}
	clusters[clusterID] = acc
	return acc
}

func accumulateCluster(acc *clusterAccumulator, item imap.MailData, hasListID bool, maxExamples int, ignore *appconfig.IgnoreConfig) {
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

	if ignore != nil {
		if matchesIgnoreMatchers(item, ignore.Watch) {
			acc.suppressWatch = true
		}
		if matchesIgnoreMatchers(item, ignore.Cleanup) {
			acc.suppressCleanup = true
		}
	}

	addExample(acc, ExampleKeySubjectRaw, strings.TrimSpace(item.SubjectRaw), maxExamples)
	for _, recipient := range item.Recipients {
		addExample(acc, ExampleKeyRecipients, recipient, maxExamples)
	}
	for _, replyTo := range item.ReplyToDomains {
		addExample(acc, ExampleKeyReplyToDomains, replyTo, maxExamples)
	}
	for _, senderDomain := range item.SenderDomains {
		addExample(acc, ExampleKeySenderDomains, senderDomain, maxExamples)
	}
	if strings.TrimSpace(item.ReturnPathDomain) != "" {
		addExample(acc, ExampleKeyReturnPathDomains, item.ReturnPathDomain, maxExamples)
	}
	for _, target := range splitAndTrim(item.ListUnsubscribeTargets) {
		addExample(acc, ExampleKeyListUnsubscribeTargets, target, maxExamples)
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

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func splitAndTrim(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
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
	case ExampleKeySubjectRaw:
		acc.examples.SubjectRaw = append(acc.examples.SubjectRaw, value)
	case ExampleKeyRecipients:
		acc.examples.Recipients = append(acc.examples.Recipients, value)
	case ExampleKeySenderDomains:
		acc.examples.SenderDomains = append(acc.examples.SenderDomains, value)
	case ExampleKeyReplyToDomains:
		acc.examples.ReplyToDomains = append(acc.examples.ReplyToDomains, value)
	case ExampleKeyListUnsubscribeTargets:
		acc.examples.ListUnsubscribeTargets = append(acc.examples.ListUnsubscribeTargets, value)
	}
}

func finalizeClusters(clusters map[string]*clusterAccumulator, options analyzeOptions) []analyzeCluster {
	minCount := options.MinCount
	if minCount <= 0 {
		minCount = 1
	}
	all := make([]analyzeCluster, 0, len(clusters))
	for clusterID, acc := range clusters {
		if acc.count < minCount {
			continue
		}
		suppressed := make([]string, 0, 2)
		if acc.suppressWatch {
			suppressed = append(suppressed, "watch")
		}
		if acc.suppressCleanup {
			suppressed = append(suppressed, "cleanup")
		}
		all = append(all, analyzeCluster{
			ClusterID:  clusterID,
			Count:      acc.count,
			LatestDate: acc.latestDate,
			Keys:       acc.keys,
			Signals: analyzeClusterSignals{
				HasListID:            acc.hasListID,
				HasListUnsubscribe:   acc.hasUnsubscribe,
				PrecedenceCategories: acc.precedence,
			},
			Examples:   acc.examples,
			Suppressed: suppressed,
		})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].ClusterID < all[j].ClusterID
	})
	if options.Top > 0 && len(all) > options.Top {
		return all[:options.Top]
	}
	return all
}
