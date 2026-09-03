package cli

import (
	"time"

	"github.com/aaronromeo/postmanpat/rulesgen"
	"github.com/spf13/cobra"
)

var rulesgenCmd = &cobra.Command{
	Use:   "rulesgen",
	Short: "Review Queue web service fed by scheduled Analyze Reports",
}

var rulesgenServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the read-only Review Queue web service",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		reports, _ := cmd.Flags().GetString("reports")
		dbPath, _ := cmd.Flags().GetString("db")
		poll, _ := cmd.Flags().GetDuration("poll")
		return rulesgen.Serve(cmd.Context(), rulesgen.ServeOptions{
			Addr:       addr,
			ReportsDir: reports,
			DBPath:     dbPath,
			PollEvery:  poll,
		})
	},
}

func init() {
	rulesgenServeCmd.Flags().String("addr", ":8092", "Listen address for the Review Queue web service")
	rulesgenServeCmd.Flags().String("reports", "", "Directory containing scheduled Analyze Report files (required)")
	rulesgenServeCmd.Flags().String("db", "", "Path to the SQLite decision store (required)")
	rulesgenServeCmd.Flags().Duration("poll", time.Minute, "How often to re-ingest the reports directory")
	_ = rulesgenServeCmd.MarkFlagRequired("reports")
	_ = rulesgenServeCmd.MarkFlagRequired("db")
	rulesgenCmd.AddCommand(rulesgenServeCmd)
}
