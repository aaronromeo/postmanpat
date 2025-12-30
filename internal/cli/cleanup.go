package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapsearch"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

const configEnvVar = "POSTMANPAT_CONFIG"
const defaultEnvFile = ".env"

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Process IMAP folders based on configured rules",
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

		if err := config.ValidateEnv(); err != nil {
			return err
		}

		cfgSummary := config.Summary(cfg)
		fmt.Fprintln(cmd.OutOrStdout(), cfgSummary)

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
			client := &imapsearch.Client{
				Addr:     fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
				Username: imapEnv.User,
				Password: imapEnv.Pass,
				Mailbox:  mailbox,
			}

			if err := client.Connect(); err != nil {
				return err
			}

			uids, err := client.SearchByMatchers(ctx, rule.Matchers)
			if closeErr := client.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Rule %q mailbox %q matched %d messages\n", rule.Name, mailbox, len(uids))
		}
		return nil
	},
}

func init() {
	cleanupCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_CONFIG)")
	cleanupCmd.Flags().Bool("dry-run", false, "Validate and report actions without making changes")
	cleanupCmd.Flags().Bool("verbose", false, "Enable verbose logging")
}

func resolveConfigPath(cmd *cobra.Command) (string, error) {
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = os.Getenv(configEnvVar)
	}
	if strings.TrimSpace(cfgPath) == "" {
		return "", errors.New("config path is required via --config or POSTMANPAT_CONFIG")
	}
	return cfgPath, nil
}

func loadEnvFile() error {
	if _, err := os.Stat(defaultEnvFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return godotenv.Load(defaultEnvFile)
}
