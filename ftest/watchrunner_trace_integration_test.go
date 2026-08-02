package ftest

import (
	"context"
	"io"
	"log/slog"
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/watchrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWatchProcessUIDsEmitsMessageSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	client, ids, cleanup := setupWatchRunnerServer(t, []string{"Archive"})
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := appconfig.Rule{
		Name:   "MoveRule",
		Client: &appconfig.ClientMatchers{SenderRegex: []string{watchSenderHostPattern}},
		Actions: []appconfig.Action{{Type: appconfig.MOVE, Destination: "Archive"}},
	}
	deps := watchrunner.Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{rule},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	state := &watchrunner.State{}
	require.NoError(t, watchrunner.ProcessUIDs(client, deps, state, []uint32{ids.NewsUID}))

	spans := rec.Ended()
	var msgSpan, actionSpan sdktrace.ReadOnlySpan
	for i := range spans {
		switch spans[i].Name() {
		case "watch.message":
			msgSpan = spans[i]
		case "watch.action":
			actionSpan = spans[i]
		}
	}
	require.NotNil(t, msgSpan, "missing watch.message span")
	require.NotNil(t, actionSpan, "missing watch.action span")

	var sawMatchedEvent bool
	for _, ev := range msgSpan.Events() {
		if ev.Name != "watch.rule_evaluated" {
			continue
		}
		for _, kv := range ev.Attributes {
			if kv.Key == attribute.Key("matched") && kv.Value.AsBool() {
				sawMatchedEvent = true
			}
		}
	}
	assert.True(t, sawMatchedEvent, "expected a matched=true rule_evaluated event")
	assert.Equal(t, msgSpan.SpanContext().SpanID(), actionSpan.Parent().SpanID(), "action should be child of message span")
}
