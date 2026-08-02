package obs

import (
	"context"
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type fakeCleanupRunner struct {
	searchErr    error
	searchResult map[string][]uint32
}

func (f *fakeCleanupRunner) Connect() error { return nil }
func (f *fakeCleanupRunner) Close() error   { return nil }
func (f *fakeCleanupRunner) SearchByServerMatchers(ctx context.Context, m appconfig.ServerMatchers) (map[string][]uint32, error) {
	return f.searchResult, f.searchErr
}
func (f *fakeCleanupRunner) MoveByMailbox(ctx context.Context, m map[string][]uint32, d string) error { return nil }
func (f *fakeCleanupRunner) DeleteByMailbox(ctx context.Context, m map[string][]uint32, e bool) error { return nil }
func (f *fakeCleanupRunner) FetchSenderDataByMailbox(ctx context.Context, m map[string][]uint32) (map[string][]imap.MailData, error) {
	return nil, nil
}

func TestWrapCleanupRunnerSearchEmitsSpanAndMetrics(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prevT, prevM := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevT)
		otel.SetMeterProvider(prevM)
	})

	inner := &fakeCleanupRunner{searchResult: map[string][]uint32{"INBOX": {7}}}
	wrapped := WrapCleanupRunner(inner)

	matched, err := wrapped.SearchByServerMatchers(context.Background(), appconfig.ServerMatchers{Folders: []string{"INBOX"}})
	require.NoError(t, err)
	assert.Equal(t, []uint32{7}, matched["INBOX"])

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "imap.search_by_server_matchers", spans[0].Name())
	assert.Equal(t, "search_by_server_matchers", valueFor(t, spans[0], "imap.operation").AsString())
	assert.Equal(t, int64(1), valueFor(t, spans[0], "imap.uid_count").AsInt64())
	assert.Equal(t, "success", valueFor(t, spans[0], "outcome").AsString())

	var out metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &out))
	assert.Equal(t, int64(1), metricSum(t, out, "postmanpat.imap.operations"))
	assert.Equal(t, 1, metricCount(t, out, "postmanpat.imap.duration"))
	assert.Equal(t, int64(0), metricSum(t, out, "postmanpat.imap.errors"))
}

func TestWrapCleanupRunnerSearchError(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prevT, prevM := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevT)
		otel.SetMeterProvider(prevM)
	})

	inner := &fakeCleanupRunner{searchErr: context.DeadlineExceeded}
	wrapped := WrapCleanupRunner(inner)

	_, err := wrapped.SearchByServerMatchers(context.Background(), appconfig.ServerMatchers{Folders: []string{"INBOX"}})
	require.Error(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "error", valueFor(t, spans[0], "outcome").AsString())
	assert.NotEmpty(t, spans[0].Events())

	var out metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &out))
	assert.Equal(t, int64(1), metricSum(t, out, "postmanpat.imap.operations"))
	assert.Equal(t, int64(1), metricSum(t, out, "postmanpat.imap.errors"))
}