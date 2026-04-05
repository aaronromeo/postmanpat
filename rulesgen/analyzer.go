package rulesgen

import (
	"context"
	"fmt"
	"time"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/aaronromeo/postmanpat/envmgr"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
)

// Analyzer handles email analysis for the rules generation server
type Analyzer struct {
	rulesConfig *envmgr.RulesConfig
	// addr        string
	// username    string
	// password    string
	// port        int
}

type IMAPConnector struct {
	Addr     string
	Username string
	Password string
	// TLSConfig             *tls.Config
	// UnilateralDataHandler *giimapclient.UnilateralDataHandler
	// Client                *giimapclient.Client
}

// NewAnalyzer creates a new analyzer with the given configuration
func NewAnalyzer(rulesCfg *envmgr.RulesConfig) *Analyzer {
	return &Analyzer{
		rulesConfig: rulesCfg,
	}
}

// DefaultAnalyzeOptions returns default analysis options
func DefaultAnalyzeOptions() analysis.Options {
	return analysis.DefaultOptions()
}

// Run executes the analysis and returns a report
func (a *Analyzer) Run(ctx context.Context, imapconnector *IMAPConnector, options analysis.Options) (*analysis.Report, error) {
	client := serverrunner.New(
		imap.WithAddr(imapconnector.Addr),
		imap.WithCreds(imapconnector.Username, imapconnector.Password),
	)

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer client.Close()

	// Run analysis on the first rule
	if len(a.rulesConfig.Rules) == 0 {
		return nil, fmt.Errorf("no rules configured")
	}

	rule := a.rulesConfig.Rules[0]
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

	arp := analysis.ReportParams{
		Mailbox:   mailbox,
		Account:   imapconnector.Username,
		Generated: time.Now().UTC(),
		AgeWindow: rule.Server.AgeWindow,
		Options:   options,
	}

	// Build the report
	report, err := a.BuildReport(data, arp)
	if err != nil {
		return nil, fmt.Errorf("failed to build report: %w", err)
	}

	return report, nil
}

func BuildTimeWindow(now time.Time, window *envmgr.AgeWindow) (analysis.TimeWindow, error) {
	after, before, err := envmgr.AgeWindowBounds(now, window)
	if err != nil {
		return analysis.TimeWindow{}, err
	}
	if before == "" {
		before = now.Format(time.RFC3339)
	}
	return analysis.TimeWindow{After: after, Before: before}, nil
}

// BuildReport creates an analysis report from mail data
func (a *Analyzer) BuildReport(data []imap.MailData, params analysis.ReportParams) (*analysis.Report, error) {
	window, err := BuildTimeWindow(params.Generated, params.AgeWindow)
	if err != nil {
		return nil, err
	}

	options := params.Options

	// Build lens clusters using shared functions from analysis package
	listLens := analysis.BuildListLens(data, options)
	senderLens := analysis.BuildSenderUnsubLens(data, options)
	templateLens := analysis.BuildTemplateLens(data, options)
	recipientTagLens := analysis.BuildRecipientTagLens(data, options)

	return &analysis.Report{
		GeneratedAt: params.Generated.Format(time.RFC3339),
		Source: analysis.Source{
			Mailbox:    params.Mailbox,
			Account:    params.Account,
			TimeWindow: analysis.TimeWindow(window),
		},
		Stats: analysis.Stats{
			TotalMessagesScanned: len(data),
		},
		Indexes: analysis.Indexes{
			ListLens:         listLens,
			SenderLens:       senderLens,
			TemplateLens:     templateLens,
			RecipientTagLens: recipientTagLens,
		},
	}, nil
}
