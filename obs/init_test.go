package obs

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInitDisabledNoop(t *testing.T) {
	// OTEL_EXPORTER_OTLP_ENDPOINT unset -> no-op shutdown, no globals installed.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := Init(context.Background())
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error: %v", err)
	}
}

func TestInitEnabledInstallsProviders(t *testing.T) {
	prevT := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prevT) })

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:9")
	shutdown, err := Init(context.Background())
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatal("expected a real sdk trace provider to be installed")
	}
	_ = shutdown(context.Background()) // export to a dead port; error ignored
}
