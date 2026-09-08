package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findCommand(cmds []*cobra.Command, name string) *cobra.Command {
	for _, cmd := range cmds {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func TestRulesgenCommandRegistered(t *testing.T) {
	rulesgen := findCommand(rootCmd.Commands(), "rulesgen")
	require.NotNil(t, rulesgen)
	require.NotNil(t, findCommand(rulesgen.Commands(), "serve"))
}

func TestRulesgenServeFlagDefaults(t *testing.T) {
	addr := rulesgenServeCmd.Flags().Lookup("addr")
	require.NotNil(t, addr)
	assert.Equal(t, ":8092", addr.DefValue)
	poll := rulesgenServeCmd.Flags().Lookup("poll")
	require.NotNil(t, poll)
	assert.Equal(t, "1m0s", poll.DefValue)
}

func TestRulesgenServeRequiresReportsAndDB(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	rootCmd.SetArgs([]string{"rulesgen", "serve"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag(s)")
	assert.Contains(t, err.Error(), "db")
	assert.Contains(t, err.Error(), "reports")
}
