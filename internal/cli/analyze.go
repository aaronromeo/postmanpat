package cli

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapclient"
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

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}

		if err := config.Validate(cfg); err != nil {
			return err
		}

		imapEnv, err := config.IMAPEnvFromEnv()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		for _, rule := range cfg.Rules {
			mailbox := rule.Matchers.Folders[0]
			client := &imapclient.Client{
				Addr:     fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
				Username: imapEnv.User,
				Password: imapEnv.Pass,
				Mailbox:  mailbox,
			}

			if err := client.Connect(); err != nil {
				return err
			}

			uids, err := client.SearchByMatchers(ctx, rule.Matchers)
			if err != nil {
				_ = client.Close()
				return err
			}

			data, err := client.FetchSenderData(ctx, uids)
			if closeErr := client.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}

			if len(data) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No sender domains found.")
				continue
			}
			writer := csv.NewWriter(os.Stdout)
			if err := writer.Write([]string{"SenderDomains", "ReplyToDomains", "Recipients", "Count"}); err != nil {
				return err
			}
			for _, iota := range data {
				if err := writer.Write([]string{iota.SenderDomains, iota.ReplyToDomains, iota.Recipients, strconv.Itoa(iota.Count)}); err != nil {
					return err
				}
			}

			// Write any buffered data to the underlying writer (standard output).
			writer.Flush()
		}

		return nil
	},
}

func init() {
	analyzeCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_CONFIG)")
	analyzeCmd.Flags().Bool("verbose", false, "Enable verbose logging")
}
