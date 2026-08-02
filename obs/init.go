package obs

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init configures the OTel SDK from OTEL_* environment variables and registers
// the trace/meter providers as OTel globals. It returns a shutdown function
// that flushes and stops the providers (safe to call multiple times). When
// OTel is disabled (no OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_SDK_DISABLED=true)
// it installs nothing and returns a no-op shutdown.
func Init(ctx context.Context) (shutdown func(context.Context) error, err error) {
	if !IsEnabled() {
		return func(context.Context) error { return nil }, nil
	}

	res, err := buildResource(ctx)
	if err != nil {
		return nil, err
	}

	traceClient := newTraceClient()
	traceExp, err := otlptracegrpc.New(ctx, traceClient)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricClient := newMetricClient()
	metricExp, err := otlpmetricgrpc.New(ctx, metricClient)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return func(shutdownCtx context.Context) error {
		var errs []error
		if err := tp.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
		if err := mp.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}, nil
}

func newTraceClient() otlptracegrpc.Option {
	host, insecure, err := otlpEndpoint()
	if err != nil || host == "" {
		return otlptracegrpc.WithEndpoint("localhost:4317")
	}
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(host)}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	headers, err := otlpHeaders()
	if err != nil || len(headers) == 0 {
		return opts[0]
	}
	opts = append(opts, otlptracegrpc.WithHeaders(headers))
	return opts[0]
}

func newMetricClient() otlpmetricgrpc.Option {
	host, insecure, err := otlpEndpoint()
	if err != nil || host == "" {
		return otlpmetricgrpc.WithEndpoint("localhost:4317")
	}
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(host)}
	if insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	headers, err := otlpHeaders()
	if err != nil || len(headers) == 0 {
		return opts[0]
	}
	opts = append(opts, otlpmetricgrpc.WithHeaders(headers))
	return opts[0]
}