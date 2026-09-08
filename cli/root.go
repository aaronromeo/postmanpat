package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const defaultConfigFile = "config.yaml"

var rootCmd = &cobra.Command{
	Use:   "postmanpat",
	Short: "postmanpat manages email cleanup and archiving",
}

// Execute runs the root command.
func Execute() {
	os.Exit(ExecuteWithContext(context.Background()))
}

// ExecuteWithContext runs the root command with context.
func ExecuteWithContext(ctx context.Context) int {
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func resolveConfigPath(cmd *cobra.Command) (string, error) {
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return "", err
	}
	if cfgPath == "" {
		cfgPath = os.Getenv(configEnvVar)
	}
	if cfgPath == "" {
		cfgPath = defaultConfigFile
	}
	return cfgPath, nil
}

func init() {
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(rulesgenCmd)
}
