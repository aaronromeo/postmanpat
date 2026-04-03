package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aaronromeo/postmanpat/envmgr"
	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/spf13/cobra"
)

func rulesgenRequiredEnvVars() []string {
	return []string{
		envmgr.EnvIMAPHost,
		envmgr.EnvIMAPPort,
		envmgr.EnvIMAPUser,
		envmgr.EnvIMAPPass,
		envmgr.EnvS3Endpoint,
		envmgr.EnvS3Region,
		envmgr.EnvS3Bucket,
		envmgr.EnvS3Key,
		envmgr.EnvS3Secret,
	}
}

var rulesgenCmd = &cobra.Command{
	Use:   "rulesgen",
	Short: "Interactive rule generation web server",
}

var rulesgenServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the rules generation web server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfgPath, err := resolveConfigPath(cmd)
		if err != nil {
			return err
		}

		if err := loadEnvFile(); err != nil {
			return err
		}

		ruleGenOutput, err := envmgr.RulesGenOutputFromEnv()
		if err != nil {
			return err
		}

		port, err := cmd.Flags().GetInt("port")
		if err != nil {
			return err
		}

		if err := envmgr.ValidateEnv(rulesgenRequiredEnvVars); err != nil {
			return err
		}

		// Allow port override from environment
		if envPort := os.Getenv("POSTMANPAT_RULESGEN_PORT"); envPort != "" {
			if p, err := strconv.Atoi(envPort); err == nil {
				port = p
			}
		}

		// Load server config from environment
		serverConfig := rulesgen.NewServerConfig(
			rulesgen.WithConfig(cfgPath),
			rulesgen.WithRulesGenStore(ruleGenOutput.StorePath),
			rulesgen.WithWatchOut(ruleGenOutput.WatchOutPath),
			rulesgen.WithCleanupOut(ruleGenOutput.CleanupOutPath),
			rulesgen.WithOneTimeOut(ruleGenOutput.OneTimeCleanupPath),
		)
		if err := serverConfig.Validate(); err != nil {
			return err
		}

		// Validate that rules have server matchers
		for _, rule := range serverConfig.Cfg.Rules {
			if rule.Server == nil {
				return fmt.Errorf("rule %q must define server matchers for rulesgen", rule.Name)
			}
		}

		// Initialize the SQLite store
		store, err := rulesgen.LoadServerStore(ruleGenOutput.StorePath)
		if err != nil {
			return fmt.Errorf("failed to initialize store: %w", err)
		}
		defer store.Close()

		server, err := rulesgen.NewServer(port, &serverConfig.Cfg, store)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Starting rules generation server on http://localhost:%d\n", port)
		fmt.Fprintf(cmd.OutOrStdout(), "API endpoint: http://localhost:%d/api/analysis\n", port)
		fmt.Fprintf(cmd.OutOrStdout(), "Clusters endpoint: http://localhost:%d/api/clusters\n", port)
		if err := server.Run(); err != nil {
			return fmt.Errorf("server error: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(rulesgenCmd)
	rulesgenCmd.AddCommand(rulesgenServeCmd)
	rulesgenServeCmd.Flags().Int("port", 8080, "Port to run the server on")
}
