package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imapclient"
	"github.com/aaronromeo/postmanpat/internal/matchers"
	imapclientv2 "github.com/emersion/go-imap/v2/imapclient"
	"github.com/spf13/cobra"
)

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
		handler := &imapclientv2.UnilateralDataHandler{
			Mailbox: func(data *imapclientv2.UnilateralDataMailbox) {
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

		selection, err := client.SelectMailbox(ctx, "INBOX")
		if err != nil {
			return err
		}

		lastCount := selection.NumMessages
		lastUID := uint32(0)
		if selection.UIDNext > 0 {
			lastUID = uint32(selection.UIDNext - 1)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "watching INBOX (messages=%d)\n", lastCount)

		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			idleCmd, err := client.Idle()
			if err != nil {
				return err
			}
			select {
			case newCount := <-updateCh:
				_ = idleCmd.Close()
				if err := idleCmd.Wait(); err != nil {
					return err
				}
				if newCount > lastCount {
					fmt.Fprintf(cmd.OutOrStdout(), "new mail detected (messages=%d)\n", newCount)
					uids, err := client.SearchUIDsNewerThan(ctx, lastUID)
					if err != nil {
						return err
					}
					if len(uids) > 0 {
						data, err := client.FetchSenderData(ctx, uids)
						if err != nil {
							return err
						}
						for _, message := range data {
							matchedAny := false
							for _, rule := range cfg.Rules {
								ok, err := matchers.MatchesClient(rule.Client, matchers.ClientMessage{
									ListID:         message.ListID,
									SenderDomains:  message.SenderDomains,
									ReplyToDomains: message.ReplyToDomains,
								})
								if err != nil {
									return err
								}
								if ok {
									matchedAny = true
									fmt.Fprintf(cmd.OutOrStdout(), "rule %q matched list_id=%q\n", rule.Name, message.ListID)
									if err := postWatchAnnouncement(rule.Name); err != nil {
										fmt.Fprintf(cmd.ErrOrStderr(), "reporting failed for rule %q: %v\n", rule.Name, err)
									}
								}
							}
							if !matchedAny {
								fmt.Fprintln(cmd.OutOrStdout(), "no rule matched")
								if err := postWatchAnnouncement(""); err != nil {
									fmt.Fprintf(cmd.ErrOrStderr(), "reporting failed for no-match: %v\n", err)
								}
							}
						}
						lastUID = maxUID(lastUID, uids)
					}
				}
				lastCount = newCount
				fmt.Fprintln(cmd.OutOrStdout(), "ready for next update")
			case <-ctx.Done():
				_ = idleCmd.Close()
				_ = idleCmd.Wait()
				return ctx.Err()
			case <-reloadTicker.C:
				updated, err := config.Load(cfgPath)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "watch config reload failed: %v\n", err)
					continue
				}
				if err := config.Validate(updated); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "watch config reload failed: %v\n", err)
					continue
				}
				if err := validateWatchRules(updated); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "watch config reload failed: %v\n", err)
					continue
				}
				cfg = updated
				fmt.Fprintln(cmd.OutOrStdout(), "watch config reloaded")
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

func maxUID(current uint32, uids []uint32) uint32 {
	max := current
	for _, uid := range uids {
		if uid > max {
			max = uid
		}
	}
	return max
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
