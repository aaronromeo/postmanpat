package watchrunner

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/aaronromeo/postmanpat/ftest"
	"github.com/aaronromeo/postmanpat/internal/config"
	"github.com/aaronromeo/postmanpat/internal/imap"
	"github.com/aaronromeo/postmanpat/internal/imap/sessionmgr"
)

func TestIsBenignIdleError(t *testing.T) {
	if !IsBenignIdleError(nil) {
		t.Fatal("expected nil error to be benign")
	}
	if !IsBenignIdleError(errString("use of closed network connection")) {
		t.Fatal("expected closed network connection error to be benign")
	}
	if IsBenignIdleError(errString("some other error")) {
		t.Fatal("expected other error to be non-benign")
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}

func TestWatchProcessUIDsMove(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, []string{"Archive"})
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := config.Rule{
		Name: "MoveRule",
		Client: &config.ClientMatchers{
			SenderRegex: []string{senderHostPattern},
		},
		Actions: []config.Action{{
			Type:        config.MOVE,
			Destination: "Archive",
		}},
	}

	deps := Deps{
		Ctx:    context.Background(),
		Client: client,
		Rules:  []config.Rule{rule},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &State{}
	if err := ProcessUIDs(deps, state, []uint32{ids.NewsUID}); err != nil {
		t.Fatalf("process uids: %v", err)
	}

	if err := assertMailboxCount(context.Background(), client, "INBOX", senderHostValue, 0); err != nil {
		t.Fatalf("inbox check: %v", err)
	}
	if err := assertMailboxCount(context.Background(), client, "Archive", senderHostValue, 1); err != nil {
		t.Fatalf("archive check: %v", err)
	}
}

func TestWatchProcessUIDsDelete(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, nil)
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := config.Rule{
		Name: "DeleteRule",
		Client: &config.ClientMatchers{
			SenderRegex: []string{senderHostPattern},
		},
		Actions: []config.Action{{
			Type: config.DELETE,
		}},
	}

	deps := Deps{
		Ctx:    context.Background(),
		Client: client,
		Rules:  []config.Rule{rule},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &State{}
	if err := ProcessUIDs(deps, state, []uint32{ids.NewsUID}); err != nil {
		t.Fatalf("process uids: %v", err)
	}

	if err := assertMailboxCount(context.Background(), client, "INBOX", senderHostValue, 0); err != nil {
		t.Fatalf("inbox check: %v", err)
	}
}

func TestWatchProcessUIDsMoveMissingDestination(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, nil)
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := config.Rule{
		Name: "MoveRule",
		Client: &config.ClientMatchers{
			SenderRegex: []string{senderHostPattern},
		},
		Actions: []config.Action{{
			Type:        config.MOVE,
			Destination: "MissingFolder",
		}},
	}

	deps := Deps{
		Ctx:    context.Background(),
		Client: client,
		Rules:  []config.Rule{rule},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &State{}
	if err := ProcessUIDs(deps, state, []uint32{ids.NewsUID}); err == nil {
		t.Fatal("expected move to missing destination to fail")
	}
}

func TestWatchProcessUIDsUnsupportedAction(t *testing.T) {
	client, ids, cleanup := setupWatchRunnerServer(t, nil)
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := config.Rule{
		Name: "UnsupportedRule",
		Client: &config.ClientMatchers{
			SenderRegex: []string{senderHostPattern},
		},
		Actions: []config.Action{{
			Type: config.ActionName("archive"),
		}},
	}

	deps := Deps{
		Ctx:    context.Background(),
		Client: client,
		Rules:  []config.Rule{rule},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &State{}
	if err := ProcessUIDs(deps, state, []uint32{ids.NewsUID}); err == nil {
		t.Fatal("expected unsupported action to fail")
	}
}

const senderHostValue = "example.com"
const senderHostPattern = "example\\.com"

func assertMailboxCount(ctx context.Context, client *imap.Client, mailbox, senderHost string, expected int) error {
	matchers := config.ServerMatchers{
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

func setupWatchRunnerServer(t *testing.T, extraMailboxes []string) (*imap.Client, ftest.MessageIDs, func()) {
	t.Helper()

	addr, ids, cleanup := ftest.SetupIMAPServer(t, nil, extraMailboxes, nil)
	opts := []sessionmgr.Option{
		sessionmgr.WithAddr(addr),
		sessionmgr.WithCreds(ftest.DefaultUser, ftest.DefaultPass),
		sessionmgr.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	}

	client := imap.New(
		opts...,
	)

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
