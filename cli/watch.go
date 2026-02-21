package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/announcer"
	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/spf13/cobra"
)

const defaultMailbox = "INBOX"

var watchTLSConfigProvider func() *tls.Config

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch the inbox for new mail (IDLE)",
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
		if err := validateWatchRules(cfg); err != nil {
			return err
		}

		testRuleName, err := cmd.Flags().GetString("test")
		if err != nil {
			return err
		}
		limit, err := cmd.Flags().GetInt("limit")
		if err != nil {
			return err
		}
		testMailbox, err := cmd.Flags().GetString("mailbox")
		if err != nil {
			return err
		}

		verbose, err := cmd.Flags().GetBool("verbose")
		if err != nil {
			return err
		}
		logLevel := slog.LevelInfo
		if verbose {
			logLevel = slog.LevelDebug
		}

		reloadTicker := time.NewTicker(5 * time.Minute)
		defer reloadTicker.Stop()

		imapEnv, err := appconfig.IMAPEnvFromEnv()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		updateCh := make(chan uint32, 1)
		handler := &giimapclient.UnilateralDataHandler{
			Mailbox: func(data *giimapclient.UnilateralDataMailbox) {
				if data.NumMessages == nil {
					return
				}
				select {
				case updateCh <- *data.NumMessages:
				default:
				}
			},
		}

		var tlsConfig *tls.Config
		if watchTLSConfigProvider != nil {
			tlsConfig = watchTLSConfigProvider()
		}

		var client watchrunner.WatchRunner = watchrunner.New(
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
			imap.WithTLSConfig(tlsConfig),
			imap.WithUnilateralDataHandler(handler),
		)
		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		out := cmd.OutOrStdout()
		logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: logLevel}))

		if strings.TrimSpace(testRuleName) != "" {
			if err := runWatchTest(cmd.Context(), client, cfg, logger, testRuleName, testMailbox, limit); err != nil {
				return err
			}
			return nil
		}

		selection, err := client.SelectMailbox(ctx, defaultMailbox)
		if err != nil {
			return err
		}

		state := &watchrunner.State{LastCount: selection.NumMessages}
		if selection.UIDNext > 0 {
			state.LastUID = uint32(selection.UIDNext - 1)
		}
		logger.Info("watching mailbox", "mailbox", "INBOX", "messages", state.LastCount, "last_uid", state.LastUID)

		var announcerService announcer.Service = announcer.New(
			announcer.WithWebhookURL(os.Getenv("POSTMANPAT_WEBHOOK_URL")),
		)

		deps := watchrunner.Deps{
			Ctx:    ctx,
			Client: client,
			Rules:  cfg.Rules,
			Log:    logger,
			Announce: func(ruleName string) {
				if err := announcerService.Do("Watch", ruleName, defaultMailbox, 1); err != nil {
					logger.Error("reporting failed", "rule", ruleName, "error", err)
				}
			},
		}

		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			idleCmd, err := client.Idle()
			if err != nil {
				if watchrunner.IsBenignIdleError(err) {
					if err := watchrunner.Reconnect(deps, state, defaultMailbox); err != nil {
						return err
					}
					continue
				}
				return err
			}
			select {
			case newCount := <-updateCh:
				logger.Debug("idle update received", "messages", newCount, "last_messages", state.LastCount)
				_ = idleCmd.Close()
				if err := idleCmd.Wait(); err != nil {
					if !watchrunner.IsBenignIdleError(err) {
						return err
					}
				}
				if newCount > state.LastCount {
					logger.Info("new mail detected", "messages", newCount)
					uids, err := client.SearchUIDsNewerThan(ctx, state.LastUID)
					if err != nil {
						return err
					}
					if err := watchrunner.ProcessUIDs(deps, state, uids); err != nil {
						return err
					}
				}
				state.LastCount = newCount
				logger.Info("ready for next update")
			case <-ctx.Done():
				if err := idleCmd.Close(); err != nil {
					logger.Error("idle close error", "error", err)
					continue
				}
				if err := idleCmd.Wait(); err != nil {
					logger.Error("idle wait error", "error", err)
					continue
				}
				return ctx.Err()
			case <-reloadTicker.C:
				logger.Debug("reload timer fired")
				_ = idleCmd.Close()
				if err := idleCmd.Wait(); err != nil {
					if !watchrunner.IsBenignIdleError(err) {
						logger.Error("watch idle close failed", "error", err)
					}
				}
				updated, err := appconfig.Load(cfgPath)
				if err != nil {
					logger.Error("watch config reload failed", "error", err)
					continue
				}
				if err := appconfig.Validate(updated); err != nil {
					logger.Error("watch config reload failed", "error", err)
					continue
				}
				if err := validateWatchRules(updated); err != nil {
					logger.Error("watch config reload failed", "error", err)
					continue
				}
				cfg = updated
				deps.Rules = updated.Rules
				logger.Info("watch config reloaded")
			}
		}
	},
}

func init() {
	watchCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_CONFIG)")
	watchCmd.Flags().Bool("verbose", false, "Enable verbose logging")
	watchCmd.Flags().String("test", "", "Run a one-off test for the named rule and exit")
	watchCmd.Flags().Int("limit", 10, "Maximum matches to return when using --test")
	watchCmd.Flags().String("mailbox", defaultMailbox, "Mailbox to scan when using --test")
}

func validateWatchRules(cfg appconfig.Config) error {
	for _, rule := range cfg.Rules {
		if rule.Server != nil {
			return fmt.Errorf("rule %q defines server matchers, which are not supported by watch", rule.Name)
		}
	}
	return nil
}

func runWatchTest(ctx context.Context, client watchrunner.WatchRunner, cfg appconfig.Config, logger *slog.Logger, ruleName, mailbox string, limit int) error {
	if strings.TrimSpace(ruleName) == "" {
		return errors.New("test rule name is required")
	}
	rule, err := findRuleByName(cfg, ruleName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(mailbox) == "" {
		mailbox = defaultMailbox
	}

	matches, err := watchrunner.RunRuleTest(ctx, client, *rule, mailbox, limit)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		logger.Info("no messages found", "mailbox", mailbox)
	}

	logger.Info("running watch test", "rule", rule.Name, "mailbox", mailbox)
	for _, match := range matches {
		logger.Info(
			"test match",
			"rule", rule.Name,
			"date", match.MessageDate,
			"subject", match.SubjectRaw,
			"list_id", match.ListID,
			"reply_to_domains", match.ReplyToDomains,
			"sender_domains", match.SenderDomains,
			"recipients", match.Recipients,
		)
	}
	logger.Info("watch test complete", "rule", rule.Name, "matches", len(matches))
	return nil
}

func findRuleByName(cfg appconfig.Config, ruleName string) (*appconfig.Rule, error) {
	for i := range cfg.Rules {
		if cfg.Rules[i].Name == ruleName {
			return &cfg.Rules[i], nil
		}
	}
	return nil, fmt.Errorf("rule %q not found", ruleName)
}
