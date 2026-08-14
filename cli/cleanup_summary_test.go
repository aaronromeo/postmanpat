package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"log/slog"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/ftest"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/obs"
	"github.com/aaronromeo/postmanpat/serverrunner"
	giimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ftest server always seeds two built-in INBOX messages:
//   - News  <news@example.com>
//   - Other <other@example.org>
//
// We add one more example.com sender, so SenderSubstring ["example.com"]
// matches exactly 2 messages.
func TestCleanupLogsCompletionSummary(t *testing.T) {
	cases := []struct {
		name   string
		dryRun bool
	}{
		{name: "live run", dryRun: false},
		{name: "dry run", dryRun: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, _, cleanup := ftest.SetupIMAPServer(t, giimap.CapSet{giimap.CapIMAP4rev1: {}}, []string{"Archive"}, []ftest.MailboxMessage{
				{
					Mailbox: "INBOX",
					From:    "sender@example.com",
					To:      "user@example.com",
					Subject: "Extra",
					Body:    "extra message",
				},
			})
			t.Cleanup(cleanup)

			cfg := &appconfig.Config{
				Rules: []appconfig.Rule{
					{
						Name: "match example.com",
						Server: &appconfig.ServerMatchers{
							Folders:         []string{"INBOX"},
							SenderSubstring: []string{"example.com"},
						},
						Actions: []appconfig.Action{
							{Type: appconfig.MOVE, Destination: "Archive"},
						},
					},
					{
						Name: "match nothing",
						Server: &appconfig.ServerMatchers{
							Folders:         []string{"INBOX"},
							SenderSubstring: []string{"nope.invalid"},
						},
						Actions: []appconfig.Action{
							{Type: appconfig.DELETE},
						},
					},
				},
			}

			var client serverrunner.ServerRunner = serverrunner.New(
				imap.WithAddr(addr),
				imap.WithCreds(ftest.DefaultUser, ftest.DefaultPass),
				imap.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
			)
			client = obs.WrapCleanupRunner(client)

			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := runCleanup(ctx, client, cfg, logger, tc.dryRun, "test-config.yaml")
			require.NoError(t, err)

			out := buf.String()
			assert.Contains(t, out, "cleanup complete")
			assert.Contains(t, out, "rules_matched=1")
			assert.Contains(t, out, "messages_matched=2")
		})
	}
}
