package cli

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/announcer"
	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/obs"
	"github.com/aaronromeo/postmanpat/watchrunner"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
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

		out := cmd.OutOrStdout()
		logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: logLevel}))

		tracer := obs.Tracer("github.com/aaronromeo/postmanpat/cli")
		meter := obs.Meter("github.com/aaronromeo/postmanpat/watchrunner")
		cycleCounter, _ := meter.Int64Counter("postmanpat.watch.cycles", metric.WithUnit("{cycle}"))
		cycleDuration, _ := meter.Float64Histogram("postmanpat.watch.cycle.duration", metric.WithUnit("s"))
		reconnectCounter, _ := meter.Int64Counter("postmanpat.watch.reconnects", metric.WithUnit("{reconnect}"))
		reloadCounter, _ := meter.Int64Counter("postmanpat.watch.config.reloads", metric.WithUnit("{reload}"))

		var client watchrunner.WatchRunner = watchrunner.New(
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
			imap.WithTLSConfig(tlsConfig),
			imap.WithUnilateralDataHandler(handler),
		)
		client = obs.WrapWatchRunner(client)

		sessionID := newSessionID()
		connectCtx, connectSpan := tracer.Start(ctx, "watch.connect",
			trace.WithAttributes(attribute.String("watch.session.id", sessionID)))

		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		if strings.TrimSpace(testRuleName) != "" {
			connectSpan.End()
			if err := runWatchTest(cmd.Context(), client, cfg, logger, testRuleName, testMailbox, limit); err != nil {
				return err
			}
			return nil
		}

		selection, err := client.SelectMailbox(connectCtx, defaultMailbox)
		if err != nil {
			connectSpan.RecordError(err)
			connectSpan.SetStatus(codes.Error, err.Error())
			connectSpan.End()
			return err
		}
		connectSpan.End()

		state := &watchrunner.State{LastCount: selection.NumMessages}
		if selection.UIDNext > 0 {
			state.LastUID = uint32(selection.UIDNext - 1)
		}
		logger.Info("watching mailbox", "mailbox", "INBOX", "messages", state.LastCount, "last_uid", state.LastUID)

		var announcerService announcer.Service = announcer.New(
			announcer.WithWebhookURL(os.Getenv("POSTMANPAT_WEBHOOK_URL")),
		)

		deps := watchrunner.Deps{
			Ctx:   ctx,
			Rules: cfg.Rules,
			Log:   logger,
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
					sessionID = newSessionID()
					rcCtx, rcSpan := tracer.Start(ctx, "watch.reconnect",
						trace.WithAttributes(attribute.String("watch.session.id", sessionID)))
					deps.Ctx = rcCtx
					if err := watchrunner.Reconnect(client, deps, state, defaultMailbox); err != nil {
						rcSpan.RecordError(err)
						rcSpan.SetStatus(codes.Error, err.Error())
						rcSpan.End()
						reconnectCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error")))
						return err
					}
					rcSpan.End()
					reconnectCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "success")))
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
					cycleCtx, cycleSpan := tracer.Start(ctx, "watch.cycle",
						trace.WithAttributes(
							attribute.String("cycle.trigger", "new_mail"),
							attribute.String("watch.session.id", sessionID),
						))
					cycleStarted := time.Now()
					logger.Info("new mail detected", "messages", newCount)
					uids, err := client.SearchUIDsNewerThan(cycleCtx, state.LastUID)
					if err != nil {
						cycleSpan.RecordError(err)
						cycleSpan.SetStatus(codes.Error, err.Error())
						cycleSpan.End()
						cycleCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						cycleDuration.Record(ctx, time.Since(cycleStarted).Seconds(), metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						return err
					}
					deps.Ctx = cycleCtx
					if err := watchrunner.ProcessUIDs(client, deps, state, uids); err != nil {
						cycleSpan.RecordError(err)
						cycleSpan.SetStatus(codes.Error, err.Error())
						cycleSpan.End()
						cycleCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						cycleDuration.Record(ctx, time.Since(cycleStarted).Seconds(), metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						return err
					}
					cycleSpan.End()
					cycleCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "success")))
					cycleDuration.Record(ctx, time.Since(cycleStarted).Seconds(), metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "success")))
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
				_, rlSpan := tracer.Start(ctx, "watch.config_reload",
					trace.WithAttributes(attribute.String("watch.session.id", sessionID)))
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
					rlSpan.End()
					reloadCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error")))
					continue
				}
				if err := appconfig.Validate(updated); err != nil {
					logger.Error("watch config reload failed", "error", err)
					rlSpan.End()
					reloadCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error")))
					continue
				}
				if err := validateWatchRules(updated); err != nil {
					logger.Error("watch config reload failed", "error", err)
					rlSpan.End()
					reloadCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error")))
					continue
				}
				cfg = updated
				deps.Rules = updated.Rules
				logger.Info("watch config reloaded")
				rlSpan.End()
				reloadCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "success")))
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

func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
