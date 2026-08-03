package obs

import (
	"testing"
)

func TestIsEnabled(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "endpoint unset", env: map[string]string{}, want: false},
		{name: "endpoint set", env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317"}, want: true},
		{name: "disabled", env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317", "OTEL_SDK_DISABLED": "true"}, want: false},
		{name: "disabled case-insensitive", env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317", "OTEL_SDK_DISABLED": "TRUE"}, want: false},
		{name: "whitespace endpoint", env: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "   "}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := IsEnabled(); got != tc.want {
				t.Fatalf("IsEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOTLPEndpoint(t *testing.T) {
	cases := []struct {
		name         string
		endpoint     string
		insecureEnv  string
		wantHost     string
		wantInsecure bool
	}{
		{name: "http scheme implies insecure", endpoint: "http://localhost:4317", wantHost: "localhost:4317", wantInsecure: true},
		{name: "https scheme stays secure", endpoint: "https://ingest.signoz.cloud:443", wantHost: "ingest.signoz.cloud:443", wantInsecure: false},
		{name: "no scheme defaults secure", endpoint: "localhost:4317", wantHost: "localhost:4317", wantInsecure: false},
		{name: "insecure env forces insecure", endpoint: "https://ingest.signoz.cloud:443", insecureEnv: "true", wantHost: "ingest.signoz.cloud:443", wantInsecure: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", tc.endpoint)
			if tc.insecureEnv == "" {
				t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
			} else {
				t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", tc.insecureEnv)
			}
			host, insecure, err := otlpEndpoint()
			if err != nil {
				t.Fatalf("otlpEndpoint() error: %v", err)
			}
			if host != tc.wantHost {
				t.Fatalf("host = %q, want %q", host, tc.wantHost)
			}
			if insecure != tc.wantInsecure {
				t.Fatalf("insecure = %v, want %v", insecure, tc.wantInsecure)
			}
		})
	}
}

func TestOTLPHeaders(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "api-key=abc, X-Other=1")
	headers, err := otlpHeaders()
	if err != nil {
		t.Fatalf("otlpHeaders() error: %v", err)
	}
	if headers["api-key"] != "abc" || headers["X-Other"] != "1" {
		t.Fatalf("unexpected headers: %v", headers)
	}
}
