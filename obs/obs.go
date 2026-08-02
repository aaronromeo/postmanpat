package obs

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Tracer returns a tracer scoped to the given instrumentation name.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Meter returns a meter scoped to the given instrumentation name.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}