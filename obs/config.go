package obs

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// IsEnabled reports whether OTel should be wired up. OTel is enabled iff
// OTEL_SDK_DISABLED is not true and OTEL_EXPORTER_OTLP_ENDPOINT is set.
func IsEnabled() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
		return false
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}

// otlpEndpoint parses OTEL_EXPORTER_OTLP_ENDPOINT into a host[:port] for the
// gRPC exporter plus whether the connection must be plaintext. A scheme of
// http:// or OTEL_EXPORTER_OTLP_INSECURE=true forces insecure; otherwise TLS.
func otlpEndpoint() (host string, insecure bool, err error) {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	insecure = insecureEnabled()
	if strings.Contains(raw, "://") {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", false, perr
		}
		if u.Host == "" {
			return "", false, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT %q has no host", raw)
		}
		if u.Scheme != "https" {
			insecure = true
		}
		return u.Host, insecure, nil
	}
	if raw == "" {
		return "", false, nil
	}
	return raw, insecure, nil
}

func insecureEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"))) {
	case "true", "1":
		return true
	default:
		return false
	}
}

// otlpHeaders parses the comma-separated OTEL_EXPORTER_OTLP_HEADERS env var.
func otlpHeaders() (map[string]string, error) {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	if raw == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid OTEL_EXPORTER_OTLP_HEADERS entry %q", pair)
		}
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	return out, nil
}
