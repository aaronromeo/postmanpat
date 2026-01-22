package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapclient"
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

		for _, rule := range cfg.Rules {
			if rule.Client != nil {
				return fmt.Errorf("rule %q defines client matchers, which are not supported by cleanup", rule.Name)
			}
			if rule.Server == nil {
				return fmt.Errorf("rule %q must define server matchers for cleanup", rule.Name)
			}
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

		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}

		client := &imapclient.Client{
			Addr:     fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			Username: imapEnv.User,
			Password: imapEnv.Pass,
		}

		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		for _, rule := range cfg.Rules {
			mailbox := rule.Server.Folders[0]
			matched, err := client.SearchByServerMatchers(ctx, *rule.Server)
			if err != nil {
				return err
			}
			uids := matched[mailbox]

			fmt.Fprintf(cmd.OutOrStdout(), "Rule %q mailbox %q matched %d messages\n", rule.Name, mailbox, len(uids))

			for _, action := range rule.Actions {
				switch action.Type {
				case config.DELETE:
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "Dry run: would delete %d messages for rule %q\n", len(uids), rule.Name)
						continue
					}
					expungeAfterDelete := true
					if action.ExpungeAfterDelete != nil {
						expungeAfterDelete = *action.ExpungeAfterDelete
					}
					if err := client.DeleteByMailbox(ctx, matched, expungeAfterDelete); err != nil {
						return err
					}
				case config.MOVE:
					if strings.TrimSpace(action.Destination) == "" {
						return fmt.Errorf("Action move missing destination: %s", rule.Name)
					}
					if dryRun {
						fmt.Fprintf(cmd.OutOrStdout(), "Dry run: would delete %d messages for rule %q\n", len(uids), rule.Name)
						continue
					}
					if err := client.MoveByMailbox(ctx, matched, strings.TrimSpace(action.Destination)); err != nil {
						return err
					}
				default:
					return fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
				}
			}
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
