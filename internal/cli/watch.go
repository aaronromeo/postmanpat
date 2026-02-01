package cli

import (
	"context"
	"fmt"

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
							for _, rule := range cfg.Rules {
								ok, err := matchers.MatchesClient(rule.Client, matchers.ClientMessage{
									ListID: message.ListID,
								})
								if err != nil {
									return err
								}
								if ok {
									fmt.Fprintf(cmd.OutOrStdout(), "rule %q matched list_id=%q\n", rule.Name, message.ListID)
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
