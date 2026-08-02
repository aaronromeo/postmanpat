package obs

import (
	"context"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
)

type cleanupRunnerWrapper struct {
	inner serverrunner.ServerRunner
	inst  imapInstruments
}

// WrapCleanupRunner returns an instrumented serverrunner.ServerRunner. Each
// interface method runs inside an imap.<op> span and emits postmanpat.imap.*
// RED metrics.
func WrapCleanupRunner(inner serverrunner.ServerRunner) serverrunner.ServerRunner {
	return &cleanupRunnerWrapper{
		inner: inner,
		inst:  newIMAPInstruments(Meter("github.com/aaronromeo/postmanpat/obs/cleanuprunner")),
	}
}

func (w *cleanupRunnerWrapper) Connect() error {
	ctx, span := startIMAPOp(context.Background(), "connect")
	started := time.Now()
	err := w.inner.Connect()
	finishIMAPOp(ctx, w.inst, "connect", "connect", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) Close() error {
	ctx, span := startIMAPOp(context.Background(), "close")
	started := time.Now()
	err := w.inner.Close()
	finishIMAPOp(ctx, w.inst, "close", "close", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) SearchByServerMatchers(ctx context.Context, matchers appconfig.ServerMatchers) (map[string][]uint32, error) {
	started := time.Now()
	mailbox := ""
	if len(matchers.Folders) > 0 {
		mailbox = matchers.Folders[0]
	}
	spanCtx, span := startIMAPOp(ctx, "search_by_server_matchers", attrMailbox.String(mailbox))
	result, err := w.inner.SearchByServerMatchers(spanCtx, matchers)
	if err == nil {
		span.SetAttributes(attrUIDCount.Int(len(result[mailbox])))
	}
	finishIMAPOp(spanCtx, w.inst, "search_by_server_matchers", "search", started, span, err)
	return result, err
}

func (w *cleanupRunnerWrapper) MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "move_by_mailbox",
		attrDestination.String(destination),
		attrUIDCount.Int(uidCount(uidsByMailbox)),
	)
	err := w.inner.MoveByMailbox(spanCtx, uidsByMailbox, destination)
	finishIMAPOp(spanCtx, w.inst, "move_by_mailbox", "move", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "delete_by_mailbox",
		attrExpunge.Bool(expunge),
		attrUIDCount.Int(uidCount(uidsByMailbox)),
	)
	err := w.inner.DeleteByMailbox(spanCtx, uidsByMailbox, expunge)
	finishIMAPOp(spanCtx, w.inst, "delete_by_mailbox", "delete", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) FetchSenderDataByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32) (map[string][]imap.MailData, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "fetch_sender_data", attrUIDCount.Int(uidCount(uidsByMailbox)))
	result, err := w.inner.FetchSenderDataByMailbox(spanCtx, uidsByMailbox)
	finishIMAPOp(spanCtx, w.inst, "fetch_sender_data", "fetch", started, span, err)
	return result, err
}

func uidCount(uidsByMailbox map[string][]uint32) int {
	total := 0
	for _, uids := range uidsByMailbox {
		total += len(uids)
	}
	return total
}