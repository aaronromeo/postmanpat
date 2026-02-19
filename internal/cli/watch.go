package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/announcer"
	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imap"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmgr"
	"github.com/aaronromeo/postmanpat/internal/matchers"
	"github.com/aaronromeo/postmanpat/internal/watchrunner"
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

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}

		if err := config.Validate(cfg); err != nil {
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

		imapEnv, err := config.IMAPEnvFromEnv()
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

		var client watchrunner.WatchRunner = imap.New(
			sessionmgr.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			sessionmgr.WithCreds(imapEnv.User, imapEnv.Pass),
			sessionmgr.WithTLSConfig(tlsConfig),
			sessionmgr.WithUnilateralDataHandler(handler),
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
				updated, err := config.Load(cfgPath)
				if err != nil {
					logger.Error("watch config reload failed", "error", err)
					continue
				}
				if err := config.Validate(updated); err != nil {
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

func validateWatchRules(cfg config.Config) error {
	for _, rule := range cfg.Rules {
		if rule.Server != nil {
			return fmt.Errorf("rule %q defines server matchers, which are not supported by watch", rule.Name)
		}
	}
	return nil
}

func runWatchTest(ctx context.Context, client watchrunner.WatchRunner, cfg config.Config, logger *slog.Logger, ruleName, mailbox string, limit int) error {
	if strings.TrimSpace(ruleName) == "" {
		return errors.New("test rule name is required")
	}
	if limit <= 0 {
		limit = 10
	}
	rule, err := findRuleByName(cfg, ruleName)
	if err != nil {
		return err
	}
	if rule.Client == nil {
		return fmt.Errorf("rule %q does not define client matchers", rule.Name)
	}
	if strings.TrimSpace(mailbox) == "" {
		mailbox = defaultMailbox
	}

	if _, err := client.SelectMailbox(ctx, mailbox); err != nil {
		return err
	}

	uids, err := client.SearchUIDsNewerThan(ctx, 0)
	if err != nil {
		return err
	}
	if len(uids) == 0 {
		logger.Info("no messages found", "mailbox", mailbox)
		return nil
	}

	logger.Info("running watch test", "rule", rule.Name, "mailbox", mailbox, "uids", len(uids))
	matches := 0
	chunkSize := 200
	for end := len(uids); end > 0 && matches < limit; end -= chunkSize {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		batch := uids[start:end]
		data, err := client.FetchSenderData(ctx, batch)
		if err != nil {
			return err
		}
		sort.Slice(data, func(i, j int) bool {
			return data[i].MessageDate.After(data[j].MessageDate)
		})
		for _, message := range data {
			ok, err := matchers.MatchesClient(rule.Client, matchers.ClientMessage{
				ListID:           message.ListID,
				SenderDomains:    message.SenderDomains,
				ReplyToDomains:   message.ReplyToDomains,
				SubjectRaw:       message.SubjectRaw,
				Recipients:       message.Recipients,
				RecipientTags:    message.RecipientTags,
				Body:             message.Body,
				Cc:               message.Cc,
				ReturnPathDomain: message.ReturnPathDomain,
				ListUnsubscribe:  message.ListUnsubscribe,
			})
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			logger.Info(
				"test match",
				"rule", rule.Name,
				"date", message.MessageDate,
				"subject", message.SubjectRaw,
				"list_id", message.ListID,
				"reply_to_domains", message.ReplyToDomains,
				"sender_domains", message.SenderDomains,
				"recipients", message.Recipients,
			)
			matches++
			if matches >= limit {
				break
			}
		}
	}
	logger.Info("watch test complete", "rule", rule.Name, "matches", matches)
	return nil
}

func findRuleByName(cfg config.Config, ruleName string) (*config.Rule, error) {
	for i := range cfg.Rules {
		if cfg.Rules[i].Name == ruleName {
			return &cfg.Rules[i], nil
		}
	}
	return nil, fmt.Errorf("rule %q not found", ruleName)
}
