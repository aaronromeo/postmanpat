package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapclient"
	"github.com/aaronromeo/postmanpat/internal/watchrunner"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"github.com/spf13/cobra"
)

const defaultMailbox = "INBOX"

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

		client := &imapclient.Client{
			Addr:                  fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			Username:              imapEnv.User,
			Password:              imapEnv.Pass,
			UnilateralDataHandler: handler,
		}
		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		selection, err := client.SelectMailbox(ctx, defaultMailbox)
		if err != nil {
			return err
		}

		state := &watchrunner.State{LastCount: selection.NumMessages}
		if selection.UIDNext > 0 {
			state.LastUID = uint32(selection.UIDNext - 1)
		}
		out := cmd.OutOrStdout()
		logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: logLevel}))
		logger.Info("watching mailbox", "mailbox", "INBOX", "messages", state.LastCount, "last_uid", state.LastUID)

		deps := watchrunner.Deps{
			Ctx:    ctx,
			Client: client,
			Rules:  cfg.Rules,
			Log:    logger,
			Announce: func(ruleName string) {
				if err := postWatchAnnouncement(ruleName); err != nil {
					logger.Error("reporting failed", "rule", ruleName, "error", err)
				}
			},
		}

		mailbox := "INBOX"
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			idleCmd, err := client.Idle()
			if err != nil {
				if watchrunner.IsBenignIdleError(err) {
					if err := watchrunner.Reconnect(deps, state, mailbox); err != nil {
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
}

func validateWatchRules(cfg config.Config) error {
	for _, rule := range cfg.Rules {
		if rule.Server != nil {
			return fmt.Errorf("rule %q defines server matchers, which are not supported by watch", rule.Name)
		}
	}
	return nil
}

func postWatchAnnouncement(ruleName string) error {
	if !config.ReportingEnabled() {
		return nil
	}
	baseURL := strings.TrimSpace(os.Getenv("POSTMANPAT_WEBHOOK_URL"))
	if baseURL == "" {
		return nil
	}
	baseURL = strings.TrimRight(baseURL, "/")
	message := "no rule matched"
	if strings.TrimSpace(ruleName) != "" {
		message = fmt.Sprintf("rule %q matched", ruleName)
	}
	payload := fmt.Sprintf("{\"message\": %q}", message)
	req, err := http.NewRequest("POST", baseURL+"/announcements", strings.NewReader(payload))
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
