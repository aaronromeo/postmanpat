package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/spf13/cobra"
)

var rulesgenCmd = &cobra.Command{
	Use:   "rulesgen",
	Short: "Interactive rule generation web server",
}

var rulesgenServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the rules generation web server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		port, err := cmd.Flags().GetInt("port")
		if err != nil {
			return err
		}

		// Allow port override from environment
		if envPort := os.Getenv("POSTMANPAT_RULESGEN_PORT"); envPort != "" {
			if p, err := strconv.Atoi(envPort); err == nil {
				port = p
			}
		}

		// Load environment file if present
		if err := loadEnvFile(); err != nil {
			return err
		}

		// Load server config from environment
		serverConfig := rulesgen.NewServerConfig(
			rulesgen.WithCleanupOut(os.Getenv("POSTMANPAT_CONFIG")),
			rulesgen.WithRulesGenStore(os.Getenv("POSTMANPAT_RULESGEN_STORE")),
			rulesgen.WithWatchOut(os.Getenv("POSTMANPAT_RULESGEN_WATCH_OUT")),
			rulesgen.WithCleanupOut(os.Getenv("POSTMANPAT_RULESGEN_CLEANUP_OUT")),
			rulesgen.WithRulesGenStore(os.Getenv("POSTMANPAT_RULESGEN_ONETIME_OUT")),
		)
		if err := serverConfig.Validate(); err != nil {
			return err
		}

		serverConfig.Port = port

		// Validate that rules have server matchers
		for _, rule := range serverConfig.Cfg.Rules {
			if rule.Server == nil {
				return fmt.Errorf("rule %q must define server matchers for rulesgen", rule.Name)
			}
		}

		server, err := rulesgen.NewServer(port, &serverConfig.Cfg)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Starting rules generation server on http://localhost:%d\n", port)
		fmt.Fprintf(cmd.OutOrStdout(), "API endpoint: http://localhost:%d/api/analysis\n", port)
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
