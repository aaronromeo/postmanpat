package obs

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func valueFor(t *testing.T, span sdktrace.ReadOnlySpan, key string) attribute.Value {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value
		}
	}
	t.Fatalf("attribute %q not found on span %q", key, span.Name())
	return attribute.Value{}
}

func metricSum(t *testing.T, out metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, sm := range out.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			s, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %q is not an int64 sum", name)
			}
			var total int64
			for _, dp := range s.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}

func metricCount(t *testing.T, out metricdata.ResourceMetrics, name string) int {
	t.Helper()
	for _, sm := range out.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric %q is not a histogram", name)
			}
			var count uint64
			for _, dp := range h.DataPoints {
				count += dp.Count
			}
			return int(count)
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}