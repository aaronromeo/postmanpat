package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"runtime/debug"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

// buildResource assembles the OTel Resource for postmanpat. Env-provided
// attributes (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES) take precedence
// over the built-in defaults.
func buildResource(ctx context.Context) (*resource.Resource, error) {
	base := resource.NewWithAttributes(
		"",
		attribute.String("service.name", "postmanpat"),
		attribute.String("service.version", serviceVersion()),
		attribute.String("service.instance.id", newInstanceID()),
		attribute.String("process.command", processCommand()),
	)

	envRes, err := resource.New(ctx, resource.WithFromEnv())
	if err != nil {
		return nil, err
	}

	// Merge(base, env): base wins over resource.Default(); env wins over base.
	merged, err := resource.Merge(base, resource.Default())
	if err != nil {
		return nil, err
	}
	return resource.Merge(merged, envRes)
}

func serviceVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func processCommand() string {
	if len(os.Args) > 1 {
		return strings.TrimSpace(os.Args[1])
	}
	return ""
}

func newInstanceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
