package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aaronromeo/postmanpat/analysis"
	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
	"github.com/spf13/cobra"
)

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

		imapEnv, err := appconfig.IMAPEnvFromEnv()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		client := serverrunner.New(
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
		)
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
		options := analysis.Options{
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
			report, err := buildAnalyzeReport(data, analyzeReportParams{
				Mailbox:   mailbox,
				Account:   imapEnv.User,
				Generated: time.Now().UTC(),
				AgeWindow: rule.Server.AgeWindow,
				Options:   options,
			})
			if err != nil {
				return err
			}

			path, err := writeAnalyzeReport(report)
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
}

type analyzeReportParams struct {
	Mailbox   string
	Account   string
	Generated time.Time
	AgeWindow *appconfig.AgeWindow
	Options   analysis.Options
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

type analyzeIndexes struct {
	// Raw              []analyzeRawRecord `json:"raw"`
	ListLens         analysis.Lens `json:"list_lens"`
	SenderLens       analysis.Lens `json:"sender_unsub_lens"`
	TemplateLens     analysis.Lens `json:"template_lens"`
	RecipientTagLens analysis.Lens `json:"recipient_tag_lens"`
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

	options := params.Options
	listLens := analysis.BuildListLens(data, options)
	senderLens := analysis.BuildSenderUnsubLens(data, options)
	templateLens := analysis.BuildTemplateLens(data, options)
	recipientTagLens := analysis.BuildRecipientTagLens(data, options)

	return analyzeReport{
		GeneratedAt: params.Generated.Format(time.RFC3339),
		Source: analyzeSource{
			Mailbox:    params.Mailbox,
			Account:    params.Account,
			TimeWindow: analyzeTimeWindow(window),
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
