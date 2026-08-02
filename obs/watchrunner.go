package obs

import (
	"context"
	"time"

	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type watchRunnerWrapper struct {
	inner watchrunner.WatchRunner
	inst  imapInstruments
}

// WrapWatchRunner returns an instrumented watchrunner.WatchRunner. Each
// interface method runs inside an imap.<op> span and emits the shared
// postmanpat.imap.* RED metrics.
func WrapWatchRunner(inner watchrunner.WatchRunner) watchrunner.WatchRunner {
	return &watchRunnerWrapper{
		inner: inner,
		inst:  newIMAPInstruments(Meter("github.com/aaronromeo/postmanpat/obs/watchrunner")),
	}
}

func (w *watchRunnerWrapper) Connect() error {
	ctx, span := startIMAPOp(context.Background(), "connect")
	started := time.Now()
	err := w.inner.Connect()
	finishIMAPOp(ctx, w.inst, "connect", "connect", started, span, err)
	return err
}

func (w *watchRunnerWrapper) Close() error {
	ctx, span := startIMAPOp(context.Background(), "close")
	started := time.Now()
	err := w.inner.Close()
	finishIMAPOp(ctx, w.inst, "close", "close", started, span, err)
	return err
}

func (w *watchRunnerWrapper) Idle() (*giimapclient.IdleCommand, error) {
	ctx, span := startIMAPOp(context.Background(), "idle.start")
	started := time.Now()
	result, err := w.inner.Idle()
	finishIMAPOp(ctx, w.inst, "idle.start", "idle_start", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) SelectMailbox(ctx context.Context, mailbox string) (*giimap.SelectData, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "select", attrMailbox.String(mailbox))
	result, err := w.inner.SelectMailbox(spanCtx, mailbox)
	finishIMAPOp(spanCtx, w.inst, "select", "select", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "fetch_sender_data", attrUIDCount.Int(len(uids)))
	result, err := w.inner.FetchSenderData(spanCtx, uids)
	finishIMAPOp(spanCtx, w.inst, "fetch_sender_data", "fetch", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "search_newer_than")
	result, err := w.inner.SearchUIDsNewerThan(spanCtx, lastUID)
	if err == nil {
		span.SetAttributes(attrUIDCount.Int(len(result)))
	}
	finishIMAPOp(spanCtx, w.inst, "search_newer_than", "search", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) MoveUIDs(ctx context.Context, uids []uint32, destination string) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "move", attrDestination.String(destination), attrUIDCount.Int(len(uids)))
	err := w.inner.MoveUIDs(spanCtx, uids, destination)
	finishIMAPOp(spanCtx, w.inst, "move", "move", started, span, err)
	return err
}

func (w *watchRunnerWrapper) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "delete", attrExpunge.Bool(expunge), attrUIDCount.Int(len(uids)))
	err := w.inner.DeleteUIDs(spanCtx, uids, expunge)
	finishIMAPOp(spanCtx, w.inst, "delete", "delete", started, span, err)
	return err
}