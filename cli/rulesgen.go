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

const defaultPort = 8000
const portFlag = "port"

var rulesgenCmd = &cobra.Command{
	Use:   "rulesgen",
	Short: "Interactive rule generation web server",
}

var rulesgenServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the rules generation web server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := loadEnvFile(); err != nil {
			return err
		}

		rulesCfgPath, err := resolveRulesConfigPath(cmd)
		if err != nil {
			return err
		}

		ruleGenOutput, err := envmgr.RulesGenOutputFromEnv()
		if err != nil {
			return err
		}

		port := defaultPort
		if cmd.Flags().Changed("port") {
			port, err = cmd.Flags().GetInt(portFlag)
			if err != nil {
				return err
			}
		}

		// Allow port override from environment
		if envPort := os.Getenv(envmgr.EnvRulesGenWebPort); envPort != "" {
			if cmd.Flags().Changed("port") {
				return fmt.Errorf("only one of %s or 'port' flag should be set", envmgr.EnvRulesGenWebPort)
			}

			if p, err := strconv.Atoi(envPort); err == nil {
				port = p
			}
		}

		imapEnv, err := envmgr.IMAPEnvFromEnv()
		if err != nil {
			return fmt.Errorf("failed to get IMAP config: %w", err)
		}

		if err := envmgr.ValidateEnv(rulesgenRequiredEnvVars); err != nil {
			return err
		}

		// Load server config from environment
		serverConfig := rulesgen.NewServerConfig(
			rulesgen.WithPort(port),
			rulesgen.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			rulesgen.WithCreds(imapEnv.User, imapEnv.Pass),
			rulesgen.WithRulesConfig(rulesCfgPath),
			rulesgen.WithRulesGenStore(ruleGenOutput.StorePath),
			rulesgen.WithWatchOut(ruleGenOutput.WatchOutPath),
			rulesgen.WithCleanupOut(ruleGenOutput.CleanupOutPath),
			rulesgen.WithOneTimeOut(ruleGenOutput.OneTimeCleanupPath),
		)
		if err := serverConfig.Validate(); err != nil {
			return err
		}

		server, err := rulesgen.NewServer(serverConfig)
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
	rulesgenServeCmd.Flags().String("config", "", "Path to YAML config file (or set POSTMANPAT_RULES_CONFIG)")
}
