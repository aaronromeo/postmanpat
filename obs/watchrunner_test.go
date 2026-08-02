package obs

import (
	"context"
	"testing"

	"github.com/aaronromeo/postmanpat/imap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type fakeWatchRunner struct{}

func (f *fakeWatchRunner) Connect() error                                   { return nil }
func (f *fakeWatchRunner) Close() error                                     { return nil }
func (f *fakeWatchRunner) Idle() (*giimapclient.IdleCommand, error)         { return nil, nil }
func (f *fakeWatchRunner) SelectMailbox(ctx context.Context, m string) (*giimap.SelectData, error) {
	return nil, nil
}
func (f *fakeWatchRunner) FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error) {
	return nil, nil
}
func (f *fakeWatchRunner) SearchUIDsNewerThan(ctx context.Context, last uint32) ([]uint32, error) {
	return nil, nil
}
func (f *fakeWatchRunner) MoveUIDs(ctx context.Context, uids []uint32, dest string) error { return nil }
func (f *fakeWatchRunner) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	return nil
}

func TestWrapWatchRunnerSelectEmitsSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	wrapped := WrapWatchRunner(&fakeWatchRunner{})
	_, err := wrapped.SelectMailbox(context.Background(), "INBOX")
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "imap.select", spans[0].Name())
	assert.Equal(t, "select", valueFor(t, spans[0], "imap.operation").AsString())
	assert.Equal(t, attribute.StringValue("INBOX"), valueFor(t, spans[0], "imap.mailbox"))
	assert.Equal(t, "success", valueFor(t, spans[0], "outcome").AsString())
}