package obs

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
)

func TestBuildResource(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "test-svc")
	res, err := buildResource(context.Background())
	if err != nil {
		t.Fatalf("buildResource() error: %v", err)
	}
	attrs := resourceAttrMap(res)
	if attrs["service.name"] != "test-svc" {
		t.Fatalf("service.name = %q, want %q", attrs["service.name"], "test-svc")
	}
	if attrs["service.version"] == "" {
		t.Fatal("expected non-empty service.version")
	}
	if attrs["service.instance.id"] == "" {
		t.Fatal("expected non-empty service.instance.id")
	}
	if attrs["process.command"] == "" {
		t.Fatal("expected non-empty process.command (test binary args)")
	}
}

func resourceAttrMap(res *resource.Resource) map[string]string {
	out := map[string]string{}
	for _, kv := range res.Attributes() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}