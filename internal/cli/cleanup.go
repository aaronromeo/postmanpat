package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/spf13/cobra"
)

const configEnvVar = "POSTMANPAT_CONFIG"

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Process IMAP folders based on configured rules",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgPath, err := resolveConfigPath(cmd)
		if err != nil {
			return err
		}

		if err := config.ValidateEnv(); err != nil {
			return err
		}

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}

		if err := config.Validate(cfg); err != nil {
			return err
		}

		cfgSummary := config.Summary(cfg)
		fmt.Fprintln(cmd.OutOrStdout(), cfgSummary)
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
