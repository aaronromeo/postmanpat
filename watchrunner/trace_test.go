package watchrunner

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	giimap "github.com/emersion/go-imap/v2"
)

type fakeRunner struct {
	fetchData []imap.MailData
}

func (f *fakeRunner) Connect() error                                   { return nil }
func (f *fakeRunner) Close() error                                     { return nil }
func (f *fakeRunner) Idle() (*giimapclient.IdleCommand, error)           { return nil, nil }
func (f *fakeRunner) SelectMailbox(ctx context.Context, m string) (*giimap.SelectData, error) {
	return nil, nil
}
func (f *fakeRunner) FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error) {
	return f.fetchData, nil
}
func (f *fakeRunner) SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error) {
	return nil, nil
}
func (f *fakeRunner) MoveUIDs(ctx context.Context, uids []uint32, dest string) error { return nil }
func (f *fakeRunner) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	return nil
}

func TestProcessUIDsEmitsRuleEvaluationTrace(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	runner := &fakeRunner{
		fetchData: []imap.MailData{
			{
				UID:     123,
				MessageID: "<test@example.com>",
				From:    []string{"sender@example.com"},
				SubjectRaw: "Test Subject",
				MessageDate: time.Date(2026, 8, 2, 15, 58, 57, 0, time.UTC),
			},
		},
	}

	deps := Deps{
		Ctx: context.Background(),
		Rules: []appconfig.Rule{
			{
				Name: "TestRule",
				Client: &appconfig.ClientMatchers{
					SubjectRegex: []string{"Test"},
				},
			},
		},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	state := &State{}
	err := ProcessUIDs(runner, deps, state, []uint32{123})
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1) // watch.message

	msgSpan := spans[0]
	assert.Equal(t, "watch.message", msgSpan.Name())
	assert.Equal(t, trace.SpanKindInternal, msgSpan.SpanKind())

	events := msgSpan.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "watch.rule_evaluated", events[0].Name)

	attrs := attrMap(msgSpan.Attributes())
	assert.Equal(t, int64(123), attrs["imap.uid"].AsInt64())
	assert.Equal(t, "<test@example.com>", attrs["email.message_id"].AsString())
	assert.Equal(t, "Test Subject", attrs["email.subject"].AsString())
	assert.Contains(t, attrs["email.internal_date"].AsString(), "2026-08-02T15:58:57Z")

	eventAttrs := attrMap(events[0].Attributes)
	assert.Equal(t, "TestRule", eventAttrs["rule.name"].AsString())
	assert.Equal(t, true, eventAttrs["matched"].AsBool())
}

func attrMap(attrs []attribute.KeyValue) map[string]attribute.Value {
	m := make(map[string]attribute.Value)
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value
	}
	return m
}