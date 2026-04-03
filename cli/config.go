package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/aaronromeo/postmanpat/envmgr"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func resolveConfigPath(cmd *cobra.Command) (string, error) {
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = os.Getenv(envmgr.RulesConfigEnvVar)
	}
	if strings.TrimSpace(cfgPath) == "" {
		return "", errors.New("config path is required via --config or POSTMANPAT_RULES_CONFIG")
	}
	return cfgPath, nil
}

func loadEnvFile() error {
	if _, err := os.Stat(envmgr.DefaultEnvFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return godotenv.Load(envmgr.DefaultEnvFile)
}
