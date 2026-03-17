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

		server, err := rulesgen.NewServer(port)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Starting rules generation server on http://localhost:%d\n", port)
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
