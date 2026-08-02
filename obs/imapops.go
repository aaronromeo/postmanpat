package obs

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	attrOperation   = attribute.Key("imap.operation")
	attrOutcome     = attribute.Key("outcome")
	attrMailbox     = attribute.Key("imap.mailbox")
	attrUIDCount    = attribute.Key("imap.uid_count")
	attrDestination = attribute.Key("imap.destination")
	attrExpunge     = attribute.Key("imap.expunge")
)

type imapInstruments struct {
	operations metric.Int64Counter
	duration   metric.Float64Histogram
	errors     metric.Int64Counter
}

func newIMAPInstruments(meter metric.Meter) imapInstruments {
	operations, _ := meter.Int64Counter(
		"postmanpat.imap.operations",
		metric.WithUnit("{op}"),
		metric.WithDescription("IMAP operations by type and outcome"),
	)
	duration, _ := meter.Float64Histogram(
		"postmanpat.imap.duration",
		metric.WithUnit("s"),
		metric.WithDescription("IMAP operation duration"),
	)
	errors, _ := meter.Int64Counter(
		"postmanpat.imap.errors",
		metric.WithUnit("{error}"),
		metric.WithDescription("IMAP operation errors"),
	)
	return imapInstruments{operations: operations, duration: duration, errors: errors}
}

// startIMAPOp starts an "imap.<operation>" span, optionally as a child of ctx.
func startIMAPOp(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	all := append(attrs, attrOperation.String(operation))
	return Tracer("github.com/aaronromeo/postmanpat/obs/imap").Start(ctx, "imap."+operation,
		trace.WithAttributes(all...))
}

// finishIMAPOp records the outcome, emits RED metrics, and ends the span.
// operation names the span; label is the short metric operation label.
func finishIMAPOp(ctx context.Context, inst imapInstruments, operation, label string, started time.Time, span trace.Span, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.SetAttributes(attrOutcome.String(outcome))
	span.End()

	attrs := []attribute.KeyValue{attrOperation.String(label), attrOutcome.String(outcome)}
	inst.operations.Add(ctx, 1, metric.WithAttributes(attrs...))
	inst.duration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attrs...))
	if err != nil {
		inst.errors.Add(ctx, 1, metric.WithAttributes(attrOperation.String(label)))
	}
}