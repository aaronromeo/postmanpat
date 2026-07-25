package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aaronromeo/postmanpat/analysis"
	"github.com/aaronromeo/postmanpat/envmgr"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/aaronromeo/postmanpat/serverrunner"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze IMAP folders and report unique sender domains",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgPath, err := resolveRulesConfigPath(cmd)
		if err != nil {
			return err
		}

		if err := loadEnvFile(); err != nil {
			return err
		}

		cfg, err := envmgr.Load(cfgPath)
		if err != nil {
			return err
		}

		if err := envmgr.Validate(cfg); err != nil {
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

		imapEnv, err := envmgr.IMAPEnvFromEnv()
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

		analyzer := rulesgen.NewAnalyzer(&cfg)

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
			report, err := analyzer.BuildReport(data, analysis.ReportParams{
				Mailbox:   mailbox,
				Account:   imapEnv.User,
				Generated: time.Now().UTC(),
				AgeWindow: rule.Server.AgeWindow,
				Options:   options,
			})
			if err != nil {
				return err
			}

			path, err := writeAnalyzeReport(*report)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
		}

		return nil
	},
}

func init() {
	analyzeCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_RULES_CONFIG)")
	analyzeCmd.Flags().Bool("verbose", false, "Enable verbose logging")
	analyzeCmd.Flags().Int("top", 100, "Maximum clusters per lens")
	analyzeCmd.Flags().Int("examples", 20, "Maximum examples per field")
	analyzeCmd.Flags().Int("min-count", 2, "Minimum cluster count to include")
}

func writeAnalyzeReport(report analysis.Report) (string, error) {
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
