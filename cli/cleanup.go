package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/obs"
	"github.com/aaronromeo/postmanpat/serverrunner"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

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

		cfg, err := appconfig.Load(cfgPath)
		if err != nil {
			return err
		}

		if err := appconfig.Validate(cfg); err != nil {
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

		if err := appconfig.ValidateEnv(); err != nil {
			return err
		}

		cfgSummary := appconfig.Summary(cfg)
		out := cmd.OutOrStdout()
		logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Info("config summary", "summary", cfgSummary)

		imapEnv, err := appconfig.IMAPEnvFromEnv()
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

		var client serverrunner.ServerRunner = serverrunner.New(
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
		)
		client = obs.WrapCleanupRunner(client)

		return runCleanup(ctx, client, &cfg, logger, dryRun, cfgPath)
	},
}

func runCleanup(ctx context.Context, client serverrunner.ServerRunner, cfg *appconfig.Config, logger *slog.Logger, dryRun bool, cfgPath string) error {
	tracer := obs.Tracer("github.com/aaronromeo/postmanpat/cli")
	meter := obs.Meter("github.com/aaronromeo/postmanpat/cleanuprunner")
	invocations, _ := meter.Int64Counter("postmanpat.cleanup.invocations", metric.WithUnit("{run}"))
	runDuration, _ := meter.Float64Histogram("postmanpat.cleanup.duration", metric.WithUnit("s"))
	ruleMatches, _ := meter.Int64Counter("postmanpat.cleanup.rule.matches", metric.WithUnit("{message}"))
	actionMessages, _ := meter.Int64Counter("postmanpat.cleanup.action.messages", metric.WithUnit("{message}"))
	actionErrors, _ := meter.Int64Counter("postmanpat.cleanup.action.errors", metric.WithUnit("{error}"))

	invCtx, invSpan := tracer.Start(ctx, "cleanup.invocation",
		trace.WithAttributes(
			attribute.String("postmanpat.command", "cleanup"),
			attribute.Bool("postmanpat.dry_run", dryRun),
			attribute.String("postmanpat.config_path", cfgPath),
			attribute.Int("postmanpat.rules.count", len(cfg.Rules)),
		))
	runStarted := time.Now()
	ctx = invCtx

	if err := client.Connect(); err != nil {
		invSpan.RecordError(err)
		invSpan.SetStatus(codes.Error, err.Error())
		invSpan.End()
		invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
		runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
		return err
	}
	defer client.Close()

	rulesMatched := 0
	messagesMatched := 0
	for _, rule := range cfg.Rules {
		mailbox := rule.Server.Folders[0]
		ruleCtx, ruleSpan := tracer.Start(ctx, "cleanup.rule",
			trace.WithAttributes(
				attribute.String("rule.name", rule.Name),
				attribute.String("rule.mailbox", mailbox),
				attribute.StringSlice("rule.actions", actionNames(rule)),
			))

		matched, err := client.SearchByServerMatchers(ruleCtx, *rule.Server)
		if err != nil {
			ruleSpan.RecordError(err)
			ruleSpan.SetStatus(codes.Error, err.Error())
			ruleSpan.End()
			invSpan.RecordError(err)
			invSpan.SetStatus(codes.Error, err.Error())
			invSpan.End()
			invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
			runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
			return err
		}
		uids := matched[mailbox]
		if len(uids) > 0 {
			rulesMatched++
		}
		messagesMatched += len(uids)
		ruleSpan.SetAttributes(attribute.Int("rule.matched_count", len(uids)))
		ruleMatches.Add(ruleCtx, int64(len(uids)), metric.WithAttributes(
			attribute.String("rule.name", rule.Name),
			attribute.String("mailbox", mailbox),
		))

		if len(uids) == 0 {
			ruleSpan.End()
			continue
		}

		dataByMailbox, err := client.FetchSenderDataByMailbox(ruleCtx, matched)
		if err != nil {
			ruleSpan.RecordError(err)
			ruleSpan.SetStatus(codes.Error, err.Error())
			ruleSpan.End()
			invSpan.RecordError(err)
			invSpan.SetStatus(codes.Error, err.Error())
			invSpan.End()
			invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
			runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
			return err
		}

		for _, action := range rule.Actions {
			actionCtx, actionSpan := tracer.Start(ruleCtx, "cleanup.action",
				trace.WithAttributes(
					attribute.String("action.type", string(action.Type)),
					attribute.String("action.destination", action.Destination),
				))

			data := dataByMailbox[mailbox]
			for i, uid := range uids {
				msg := data[i]
				actionSpan.AddEvent("action.message_identified",
					trace.WithAttributes(
						attribute.Int64("imap.uid", int64(uid)),
						attribute.String("email.message_id", msg.MessageID),
						attribute.String("email.from", strings.Join(msg.From, ",")),
						attribute.String("email.subject", msg.SubjectRaw),
						attribute.String("email.internal_date", msg.MessageDate.Format(time.RFC3339)),
					))
			}

			actionSpan.AddEvent("action.applied",
				trace.WithAttributes(
					attribute.Int("action.uid_count", len(uids)),
					attribute.Bool("action.dry_run", dryRun),
				))

			switch action.Type {
			case appconfig.DELETE:
				if dryRun {
					logger.Info("dry run delete", "rule", rule.Name, "messages", len(uids))
					actionSpan.End()
					continue
				}
				expungeAfterDelete := true
				if action.ExpungeAfterDelete != nil {
					expungeAfterDelete = *action.ExpungeAfterDelete
				}
				if err := client.DeleteByMailbox(actionCtx, matched, expungeAfterDelete); err != nil {
					actionSpan.RecordError(err)
					actionSpan.SetStatus(codes.Error, err.Error())
					actionSpan.End()
					actionErrors.Add(actionCtx, 1, metric.WithAttributes(
						attribute.String("action.type", string(action.Type)),
						attribute.String("rule.name", rule.Name),
					))
					invSpan.RecordError(err)
					invSpan.SetStatus(codes.Error, err.Error())
					invSpan.End()
					invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
					runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
					return err
				}
				actionMessages.Add(actionCtx, int64(len(uids)), metric.WithAttributes(
					attribute.String("action.type", string(action.Type)),
					attribute.String("rule.name", rule.Name),
					attribute.String("destination", ""),
					attribute.Bool("postmanpat.dry_run", dryRun),
				))
			case appconfig.MOVE:
				if strings.TrimSpace(action.Destination) == "" {
					err := fmt.Errorf("Action move missing destination: %s", rule.Name)
					actionSpan.RecordError(err)
					actionSpan.SetStatus(codes.Error, err.Error())
					actionSpan.End()
					actionErrors.Add(actionCtx, 1, metric.WithAttributes(
						attribute.String("action.type", string(action.Type)),
						attribute.String("rule.name", rule.Name),
					))
					invSpan.RecordError(err)
					invSpan.SetStatus(codes.Error, err.Error())
					invSpan.End()
					invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
					runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
					return err
				}
				if dryRun {
					logger.Info("dry run move", "rule", rule.Name, "messages", len(uids))
					actionSpan.End()
					continue
				}
				if err := client.MoveByMailbox(actionCtx, matched, strings.TrimSpace(action.Destination)); err != nil {
					actionSpan.RecordError(err)
					actionSpan.SetStatus(codes.Error, err.Error())
					actionSpan.End()
					actionErrors.Add(actionCtx, 1, metric.WithAttributes(
						attribute.String("action.type", string(action.Type)),
						attribute.String("rule.name", rule.Name),
					))
					invSpan.RecordError(err)
					invSpan.SetStatus(codes.Error, err.Error())
					invSpan.End()
					invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
					runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
					return err
				}
				actionMessages.Add(actionCtx, int64(len(uids)), metric.WithAttributes(
					attribute.String("action.type", string(action.Type)),
					attribute.String("rule.name", rule.Name),
					attribute.String("destination", action.Destination),
					attribute.Bool("postmanpat.dry_run", dryRun),
				))
			default:
				err := fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
				actionSpan.RecordError(err)
				actionSpan.SetStatus(codes.Error, err.Error())
				actionSpan.End()
				actionErrors.Add(actionCtx, 1, metric.WithAttributes(
					attribute.String("action.type", string(action.Type)),
					attribute.String("rule.name", rule.Name),
				))
				invSpan.RecordError(err)
				invSpan.SetStatus(codes.Error, err.Error())
				invSpan.End()
				invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
				runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
				return err
			}
			actionSpan.End()
		}
		ruleSpan.End()
	}

	invSpan.SetAttributes(
		attribute.Int("postmanpat.rules.matched", rulesMatched),
		attribute.Int("postmanpat.messages.matched", messagesMatched),
	)
	invSpan.End()
	invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "success"), attribute.Bool("postmanpat.dry_run", dryRun)))
	runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "success"), attribute.Bool("postmanpat.dry_run", dryRun)))
	logger.Info("cleanup complete",
		"rules_matched", rulesMatched,
		"messages_matched", messagesMatched,
		"dry_run", dryRun,
	)
	return nil
}

func init() {
	cleanupCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_CONFIG)")
	cleanupCmd.Flags().Bool("dry-run", false, "Validate and report actions without making changes")
}

func actionNames(rule appconfig.Rule) []string {
	names := make([]string, len(rule.Actions))
	for i, action := range rule.Actions {
		names[i] = string(action.Type)
	}
	return names
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
