package ftest

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner"
)

const watchSenderHostValue = "example.com"
const watchSenderHostPattern = "example\\.com"

func TestWatchProcessUIDsMove(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, []string{"Archive"})
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := appconfig.Rule{
		Name: "MoveRule",
		Client: &appconfig.ClientMatchers{
			SenderRegex: []string{watchSenderHostPattern},
		},
		Actions: []appconfig.Action{{
			Type:        appconfig.MOVE,
			Destination: "Archive",
		}},
	}

	deps := watchrunner.Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{rule},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &watchrunner.State{}
	if err := watchrunner.ProcessUIDs(client, deps, state, []uint32{ids.NewsUID}); err != nil {
		t.Fatalf("process uids: %v", err)
	}

	if err := assertWatchMailboxCount(context.Background(), client, "INBOX", watchSenderHostValue, 0); err != nil {
		t.Fatalf("inbox check: %v", err)
	}
	if err := assertWatchMailboxCount(context.Background(), client, "Archive", watchSenderHostValue, 1); err != nil {
		t.Fatalf("archive check: %v", err)
	}
}

func TestWatchProcessUIDsDelete(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, nil)
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := appconfig.Rule{
		Name: "DeleteRule",
		Client: &appconfig.ClientMatchers{
			SenderRegex: []string{watchSenderHostPattern},
		},
		Actions: []appconfig.Action{{
			Type: appconfig.DELETE,
		}},
	}

	deps := watchrunner.Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{rule},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &watchrunner.State{}
	if err := watchrunner.ProcessUIDs(client, deps, state, []uint32{ids.NewsUID}); err != nil {
		t.Fatalf("process uids: %v", err)
	}

	if err := assertWatchMailboxCount(context.Background(), client, "INBOX", watchSenderHostValue, 0); err != nil {
		t.Fatalf("inbox check: %v", err)
	}
}

func TestWatchProcessUIDsMoveMissingDestination(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, nil)
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := appconfig.Rule{
		Name: "MoveRule",
		Client: &appconfig.ClientMatchers{
			SenderRegex: []string{watchSenderHostPattern},
		},
		Actions: []appconfig.Action{{
			Type:        appconfig.MOVE,
			Destination: "MissingFolder",
		}},
	}

	deps := watchrunner.Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{rule},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &watchrunner.State{}
	if err := watchrunner.ProcessUIDs(client, deps, state, []uint32{ids.NewsUID}); err == nil {
		t.Fatal("expected move to missing destination to fail")
	}
}

func TestWatchProcessUIDsUnsupportedAction(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, nil)
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := appconfig.Rule{
		Name: "UnsupportedRule",
		Client: &appconfig.ClientMatchers{
			SenderRegex: []string{watchSenderHostPattern},
		},
		Actions: []appconfig.Action{{
			Type: appconfig.ActionName("archive"),
		}},
	}

	deps := watchrunner.Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{rule},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &watchrunner.State{}
	if err := watchrunner.ProcessUIDs(client, deps, state, []uint32{ids.NewsUID}); err == nil {
		t.Fatal("expected unsupported action to fail")
	}
}

func assertWatchMailboxCount(ctx context.Context, client *watchrunner.Client, mailbox, senderHost string, expected int) error {
	matchers := appconfig.ServerMatchers{
		Folders:         []string{mailbox},
		SenderSubstring: []string{senderHost},
	}
	matched, err := client.SearchByServerMatchers(ctx, matchers)
	if err != nil {
		return err
	}
	if len(matched[mailbox]) != expected {
		return fmt.Errorf("expected %d messages in %s, got %d", expected, mailbox, len(matched[mailbox]))
	}
	return nil
}

func setupWatchRunnerServer(t *testing.T, extraMailboxes []string) (*watchrunner.Client, MessageIDs, func()) {
	t.Helper()

	addr, ids, cleanup := SetupIMAPServer(t, nil, extraMailboxes, nil)
	opts := []imap.Option{
		imap.WithAddr(addr),
		imap.WithCreds(DefaultUser, DefaultPass),
		imap.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	}

	client := watchrunner.New(opts...)

	if err := client.Connect(); err != nil {
		cleanup()
		t.Fatalf("connect: %v", err)
	}

	combinedCleanup := func() {
		_ = client.Close()
		cleanup()
	}
	return client, ids, combinedCleanup
}
