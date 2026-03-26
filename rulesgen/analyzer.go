package rulesgen

import (
	"context"
	"fmt"
	"time"

	"github.com/aaronromeo/postmanpat/analysis"
	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
)

// Analyzer handles email analysis for the rules generation server
type Analyzer struct {
	config *appconfig.Config
}

// NewAnalyzer creates a new analyzer with the given configuration
func NewAnalyzer(cfg *appconfig.Config) *Analyzer {
	return &Analyzer{
		config: cfg,
	}
}

// analysisReport represents the results of an analysis run
type analysisReport struct {
	GeneratedAt string          `json:"generated_at"`
	Source      analysisSource  `json:"source"`
	Stats       analysisStats   `json:"stats"`
	Indexes     analysisIndexes `json:"indexes"`
}

// analysisSource contains metadata about the analysis source
type analysisSource struct {
	Mailbox    string             `json:"mailbox"`
	Account    string             `json:"account"`
	TimeWindow analysisTimeWindow `json:"time_window"`
}

// analysisTimeWindow represents the time range analyzed
type analysisTimeWindow struct {
	After  string `json:"after"`
	Before string `json:"before"`
}

// analysisStats contains statistics about the analysis
type analysisStats struct {
	TotalMessagesScanned int `json:"total_messages_scanned"`
}

// analysisIndexes contains all lens-based cluster indexes
type analysisIndexes struct {
	ListLens         analysis.Lens `json:"list_lens"`
	SenderLens       analysis.Lens `json:"sender_unsub_lens"`
	TemplateLens     analysis.Lens `json:"template_lens"`
	RecipientTagLens analysis.Lens `json:"recipient_tag_lens"`
}

// DefaultAnalyzeOptions returns default analysis options
func DefaultAnalyzeOptions() analysis.Options {
	return analysis.DefaultOptions()
}

// Run executes the analysis and returns a report
func (a *Analyzer) Run(ctx context.Context, options analysis.Options) (*analysisReport, error) {
	imapEnv, err := appconfig.IMAPEnvFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to get IMAP config: %w", err)
	}

	client := serverrunner.New(
		imap.WithAddr(fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port)),
		imap.WithCreds(imapEnv.User, imapEnv.Pass),
	)

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer client.Close()

	// Run analysis on the first rule
	if len(a.config.Rules) == 0 {
		return nil, fmt.Errorf("no rules configured")
	}

	rule := a.config.Rules[0]
	if rule.Server == nil {
		return nil, fmt.Errorf("rule %q has no server matchers", rule.Name)
	}

	mailbox := rule.Server.Folders[0]

	matched, err := client.SearchByServerMatchers(ctx, *rule.Server)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}

	dataByMailbox, err := client.FetchSenderDataByMailbox(ctx, matched)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sender data: %w", err)
	}

	data := dataByMailbox[mailbox]

	// Build the report
	report, err := a.buildReport(data, mailbox, imapEnv.User, rule.Server.AgeWindow, options)
	if err != nil {
		return nil, fmt.Errorf("failed to build report: %w", err)
	}

	return report, nil
}

// buildReport creates an analysis report from mail data
func (a *Analyzer) buildReport(data []imap.MailData, mailbox, account string, ageWindow *appconfig.AgeWindow, options analysis.Options) (*analysisReport, error) {
	now := time.Now().UTC()

	// Build time window
	after, before, err := appconfig.AgeWindowBounds(now, ageWindow)
	if err != nil {
		return nil, err
	}
	if before == "" {
		before = now.Format(time.RFC3339)
	}

	// Build lens clusters using shared functions from analysis package
	listLens := analysis.BuildListLens(data, options)
	senderLens := analysis.BuildSenderUnsubLens(data, options)
	templateLens := analysis.BuildTemplateLens(data, options)
	recipientTagLens := analysis.BuildRecipientTagLens(data, options)

	return &analysisReport{
		GeneratedAt: now.Format(time.RFC3339),
		Source: analysisSource{
			Mailbox: mailbox,
			Account: account,
			TimeWindow: analysisTimeWindow{
				After:  after,
				Before: before,
			},
		},
		Stats: analysisStats{
			TotalMessagesScanned: len(data),
		},
		Indexes: analysisIndexes{
			ListLens:         listLens,
			SenderLens:       senderLens,
			TemplateLens:     templateLens,
			RecipientTagLens: recipientTagLens,
		},
	}, nil
}
