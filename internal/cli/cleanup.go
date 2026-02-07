package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapclient"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

const configEnvVar = "POSTMANPAT_CONFIG"
const defaultEnvFile = ".env"
const webhookAnnouncePath = "/announcements"

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
		out := cmd.OutOrStdout()
		logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Info("config summary", "summary", cfgSummary)

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

			logger.Info("rule matched", "rule", rule.Name, "mailbox", mailbox, "messages", len(uids))
			if len(uids) > 0 {
				if err := postAnnouncement(rule.Name, mailbox, len(uids)); err != nil {
					logger.Error("reporting failed", "rule", rule.Name, "mailbox", mailbox, "error", err)
				}
			}

			for _, action := range rule.Actions {
				switch action.Type {
				case config.DELETE:
					if dryRun {
						logger.Info("dry run delete", "rule", rule.Name, "messages", len(uids))
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
						logger.Info("dry run move", "rule", rule.Name, "messages", len(uids))
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

func postAnnouncement(ruleName, mailbox string, count int) error {
	if !config.ReportingEnabled() {
		return nil
	}
	baseURL := strings.TrimSpace(os.Getenv("POSTMANPAT_WEBHOOK_URL"))
	if baseURL == "" {
		return nil
	}
	baseURL = strings.TrimRight(baseURL, "/")
	message := fmt.Sprintf("Rule %q mailbox %q matched %d messages\n", ruleName, mailbox, count)
	payload := fmt.Sprintf("{\"message\": %q}", message)
	req, err := http.NewRequest("POST", baseURL+webhookAnnouncePath, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reporting webhook returned status %s", resp.Status)
	}
	return nil
}
