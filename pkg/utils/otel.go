package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"aaronromeo.com/postmanpat/pkg/base"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/encoding/gzip"
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	resource, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			attribute.String("service.name", base.UPTRACE_SERVICE),
			attribute.String("service.version", "1.0.0"),
		))
	if err != nil {
		handleErr(err)
		return
	}

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(ctx, resource)
	if err != nil {
		handleErr(err)
		return
	}

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx, resource)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceExporter(ctx context.Context) (trace.SpanExporter, error) {
	dsn := os.Getenv(base.UPTRACE_DSN_ENV_VAR)
	if dsn == "" {
		panic(fmt.Sprintf("%s environment variable is required", base.UPTRACE_DSN_ENV_VAR))
	}
	// fmt.Println("using DSN:", dsn)

	return otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint("otlp.uptrace.dev"),
		otlptracehttp.WithHeaders(map[string]string{
			// Set the Uptrace DSN here or use UPTRACE_DSN env var.
			"uptrace-dsn": dsn,
		}),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	)
}

func newTraceProvider(ctx context.Context, resource *resource.Resource) (*trace.TracerProvider, error) {
	traceExporter, err := newTraceExporter(ctx)
	if err != nil {
		return nil, err
	}

	bsp := trace.NewBatchSpanProcessor(traceExporter,
		trace.WithMaxQueueSize(10_000),
		trace.WithMaxExportBatchSize(10_000))

	traceProvider := trace.NewTracerProvider(
		trace.WithResource(resource),
		trace.WithIDGenerator(xray.NewIDGenerator()),
		trace.WithSpanProcessor(bsp),
		trace.WithBatcher(traceExporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			trace.WithBatchTimeout(time.Second)),
	)
	return traceProvider, nil
}

func newMeterExporter(ctx context.Context) (*otlpmetricgrpc.Exporter, error) {
	dsn := os.Getenv(base.UPTRACE_DSN_ENV_VAR)
	if dsn == "" {
		panic(fmt.Sprintf("%s environment variable is required", base.UPTRACE_DSN_ENV_VAR))
	}
	// fmt.Println("using DSN:", dsn)

	preferDeltaTemporalitySelector := func(kind metric.InstrumentKind) metricdata.Temporality {
		switch kind {
		case metric.InstrumentKindCounter,
			metric.InstrumentKindObservableCounter,
			metric.InstrumentKindHistogram:
			return metricdata.DeltaTemporality
		default:
			return metricdata.CumulativeTemporality
		}
	}

	return otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("otlp.uptrace.dev:4317"),
		otlpmetricgrpc.WithHeaders(map[string]string{
			// Set the Uptrace DSN here or use UPTRACE_DSN env var.
			"uptrace-dsn": dsn,
		}),
		otlpmetricgrpc.WithCompressor(gzip.Name),
		otlpmetricgrpc.WithTemporalitySelector(preferDeltaTemporalitySelector),
	)
}

func newMeterProvider(ctx context.Context, _ *resource.Resource) (*metric.MeterProvider, error) {
	metricExporter, err := newMeterExporter(ctx)
	if err != nil {
		return nil, err
	}

	reader := metric.NewPeriodicReader(
		metricExporter,
		metric.WithInterval(15*time.Second),
	)

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(reader),
	)
	return meterProvider, nil
}

func newLoggerExporter(ctx context.Context) (*otlploghttp.Exporter, error) {
	dsn := os.Getenv(base.UPTRACE_DSN_ENV_VAR)
	if dsn == "" {
		panic(fmt.Sprintf("%s environment variable is required", base.UPTRACE_DSN_ENV_VAR))
	}
	// fmt.Println("using DSN:", dsn)

	return otlploghttp.New(ctx,
		otlploghttp.WithEndpoint("otlp.uptrace.dev"),
		otlploghttp.WithHeaders(map[string]string{
			"uptrace-dsn": dsn,
		}),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
	)
}

func newLoggerProvider(ctx context.Context) (*log.LoggerProvider, error) {
	logExporter, err := newLoggerExporter(ctx)
	if err != nil {
		return nil, err
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
	return loggerProvider, nil
}
