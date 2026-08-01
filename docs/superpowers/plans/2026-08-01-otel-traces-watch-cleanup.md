# OTel Traces for Watch & Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit an OpenTelemetry trace when the `watch` command receives an email (showing every rule it was compared against and whether it matched) and one root trace per `cleanup` run (showing which rules matched emails and which actions ran), plus RED metrics on IMAP calls, all testable and exportable to a self-hosted SigNoz instance over plaintext OTLP gRPC.

**Architecture:** A new `obs/` package owns provider construction, env-driven config, and `WrapCleanupRunner`/`WrapWatchRunner` decorators that wrap the IMAP runner interfaces (`serverrunner.ServerRunner`, `watchrunner.WatchRunner`) so each IMAP call becomes an `imap.<op>` child span plus RED metrics. Domain spans (`cleanup.invocation`, `cleanup.rule`, `cleanup.action`, `watch.cycle`, `watch.message`, `watch.action`) are emitted from the CLI/runner code directly via `otel.Tracer`/`otel.Meter`. `cmd/postmanpat/main.go` calls `obs.Init(ctx)` and defers shutdown. OTel is off (no-op providers) unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set; self-hosted SigNoz plaintext support is added via `OTEL_EXPORTER_OTLP_INSECURE` / `http://` scheme detection (documented deviation from the original spec §4.3). The slog→OTel logs bridge is **deferred** to a follow-up plan (user chose traces + metrics).

**Tech Stack:** Go 1.25.5, `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/sdk`, `go.opentelemetry.io/otel/sdk/trace`, `go.opentelemetry.io/otel/sdk/metric`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`, `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`. Tests use `sdktrace.NewTracerProvider` + `tracetest.NewSpanRecorder`, `sdkmetric.NewManualReader`, the existing in-memory TLS IMAP server in `ftest/`, and `stretchr/testify`. SigNoz self-hosted on the same machine; OTLP gRPC on `:4317` plaintext.

---

## Canonical span / attribute / metric contract

Used consistently by every task. Do not rename these.

**Span names**
| Where | Span name |
|---|---|
| obs decorator (cleanup) | `imap.connect`, `imap.close`, `imap.search_by_server_matchers`, `imap.move_by_mailbox`, `imap.delete_by_mailbox`, `imap.fetch_sender_data` |
| obs decorator (watch) | `imap.connect`, `imap.close`, `imap.idle.start`, `imap.select`, `imap.fetch_sender_data`, `imap.search_newer_than`, `imap.move`, `imap.delete` |
| cli/cleanup.go | `cleanup.invocation`, `cleanup.rule`, `cleanup.action` |
| cli/watch.go | `watch.connect`, `watch.reconnect`, `watch.config_reload`, `watch.cycle` |
| watchrunner/runner.go | `watch.message`, `watch.action` |

**Span events**
- On `cleanup.action`: `action.message_identified` (one per matched email; attrs `imap.uid`, `email.message_id`, `email.from`, `email.subject`, `email.internal_date`) and `action.applied` (attrs `action.uid_count`, `action.dry_run`).
- On `watch.message`: `watch.rule_evaluated` (one per rule; attrs `rule.name`, `matched`).

**Attribute keys**
- Span attrs: `postmanpat.command`, `postmanpat.dry_run`, `postmanpat.config_path`, `postmanpat.rules.count`, `postmanpat.rules.matched`, `postmanpat.messages.matched`, `rule.name`, `rule.mailbox`, `rule.actions`, `rule.matched_count`, `action.type`, `action.destination`, `action.dry_run`, `action.uid_count`, `imap.operation`, `imap.mailbox`, `imap.uid_count`, `imap.destination`, `imap.expunge`, `imap.uid`, `email.message_id`, `email.from`, `email.subject`, `email.internal_date`, `watch.session.id`, `cycle.trigger`, `matched`, `outcome`.

**Metrics**
| Instrument | Type | Unit | Attrs |
|---|---|---|---|
| `postmanpat.imap.operations` | Counter | `{op}` | `imap.operation`, `outcome` |
| `postmanpat.imap.duration` | Histogram | `s` | `imap.operation`, `outcome` |
| `postmanpat.imap.errors` | Counter | `{error}` | `imap.operation` |
| `postmanpat.cleanup.invocations` | Counter | `{run}` | `outcome`, `postmanpat.dry_run` |
| `postmanpat.cleanup.duration` | Histogram | `s` | `outcome`, `postmanpat.dry_run` |
| `postmanpat.cleanup.rule.matches` | Counter | `{message}` | `rule.name`, `mailbox` |
| `postmanpat.cleanup.action.messages` | Counter | `{message}` | `action.type`, `rule.name`, `destination`, `postmanpat.dry_run` |
| `postmanpat.cleanup.action.errors` | Counter | `{error}` | `action.type`, `rule.name` |
| `postmanpat.watch.cycles` | Counter | `{cycle}` | `trigger`, `outcome` |
| `postmanpat.watch.cycle.duration` | Histogram | `s` | `trigger`, `outcome` |
| `postmanpat.watch.messages.processed` | Counter | `{message}` | — |
| `postmanpat.watch.rule.matches` | Counter | `{message}` | `rule.name` |
| `postmanpat.watch.action.messages` | Counter | `{message}` | `action.type`, `rule.name`, `destination` |
| `postmanpat.watch.reconnects` | Counter | `{reconnect}` | `outcome` |
| `postmanpat.watch.config.reloads` | Counter | `{reload}` | `outcome` |

**Known documented deviations from the 2026-06-13 spec** (applied in Task 2):
1. `OTEL_EXPORTER_OTLP_INSECURE` is honored (spec §4.3 said "not honored") because self-hosted SigNoz's OTLP gRPC receiver is plaintext.
2. The slog→OTel logs bridge (§3.3) is deferred. No `otelslog`/`otlploggrpc` deps.
3. `Connect()`/`Close()`/`Idle()` take no context, so their decorator spans are root spans (background ctx), not children of the invocation/cycle span. The domain spans (`cleanup.invocation`, `watch.connect`) still wrap them in the trace timeline.

---

## Phase 1 — Foundation (`obs` package)

### Task 1: Add OpenTelemetry dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add OTel modules**

Run:
```bash
go get go.opentelemetry.io/otel@latest \
  go.opentelemetry.io/otel/sdk@latest \
  go.opentelemetry.io/otel/sdk/metric@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlpmetric@latest \
  go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc@latest
```

Expected: `go.mod` gains `go.opentelemetry.io/otel`, `.../sdk`, `.../sdk/metric`, the two exporter modules and their transitive deps (`go.opentelemetry.io/otel/semconv`, `google.golang.org/grpc`, etc.). No errors.

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: exits 0 (nothing imports the new modules yet).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add OpenTelemetry SDK and OTLP exporter dependencies"
```

---

### Task 2: Update the OTel design spec

**Files:**
- Modify: `docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md`

- [ ] **Step 1: Record deviations and fill pending sections**

Make these edits to the spec:

1. Header: change `**Status:** Draft (in progress)` to `**Status:** Approved — implementation plan: docs/superpowers/plans/2026-08-01-otel-traces-watch-cleanup.md`. Add `**Date updated:** 2026-08-01`.
2. In §4.2's table, add the row:
   ```markdown
   | `OTEL_EXPORTER_OTLP_INSECURE` | Force plaintext gRPC (no TLS) | `true` for self-hosted SigNoz; also implied when the endpoint scheme is `http://` |
   ```
3. Replace the first bullet of §4.3 (`OTEL_EXPORTER_OTLP_INSECURE` — not honored) with:
   ```markdown
   - **`OTEL_EXPORTER_OTLP_INSECURE`** — honored in this plan (deviation). Self-hosted SigNoz's OTLP gRPC receiver (`:4317`) is plaintext by default, so TLS-only would break the primary deployment target.
   ```
4. §3.3 (`Logs`): prepend `> DEFERRED to a follow-up plan — the slog→OTel bridge is out of scope for the 2026-08-01 implementation plan (traces + metrics only).`
5. Replace the placeholder section §5 `## 5. Resource attribution (pending)` with:
   ```markdown
   ## 5. Resource attribution

   - `service.name=postmanpat` (overridable via `OTEL_SERVICE_NAME`).
   - `service.version` from `debug.ReadBuildInfo()` main module version; `dev` fallback.
   - `service.instance.id` = random 128-bit hex per process (crypto/rand).
   - `process.command` = `os.Args[1]` (e.g. `cleanup`, `watch`).
   - `OTEL_RESOURCE_ATTRIBUTES` merged in via `resource.WithFromEnv()`; env wins over defaults.
   ```
6. Replace the placeholder section §6 `## 6. Testing strategy (pending)` with:
   ```markdown
   ## 6. Testing strategy

   - `obs` unit tests: in-memory span recorder (`tracetest.NewSpanRecorder`) + manual metric reader (`sdkmetric.NewManualReader`); fakes for the runner interfaces; `otel.SetTracerProvider`/`SetMeterProvider` swapped in tests and restored.
   - Watch message processing: unit test `watchrunner.ProcessUIDs` against a fake runner asserting `watch.message` span + `watch.rule_evaluated` events; `ftest/` integration against the in-memory TLS IMAP server.
   - Cleanup: full `cleanup` command execution (cobra) against the in-memory IMAP server with an in-memory tracer, asserting `cleanup.invocation`→`cleanup.rule`→`cleanup.action` parenting and `action.message_identified` events.
   - The IDLE loop itself (`cli/watch.go` select on `updateCh`/`ctx.Done()`/`reloadTicker.C`) is not unit-testable; its span-shaping pieces (search, ProcessUIDs) are covered by the above, and the loop is verified manually against SigNoz.
   ```
7. Replace the placeholder section §7 `## 7. Out of scope (pending)` with:
   ```markdown
   ## 7. Out of scope

   - slog→OTel logs bridge and OTLP logs export (deferred).
   - OTLP/HTTP exporter (gRPC only for v1).
   - Per-message *spans* for cleanup (events are used to bound cardinality on large initial runs).
   - S3 archival instrumentation (`obs.WrapArchiveClient` seam remains documented but unused).
   ```
8. §1 Key-decisions table: change the "Metrics scope" row value to `RED (rate/errors/duration) per IMAP operation + domain counters (logs bridge deferred)`.

- [ ] **Step 2: Review the diff**

Run: `git diff --stat docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md`
Expected: the file is modified; nothing else.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md
git commit -m "docs: finalize OTel spec for traces+metrics plan, document INSECURE deviation"
```

---

### Task 3: `obs/config.go` — enablement and OTLP endpoint parsing

**Files:**
- Create: `obs/config.go`
- Test: `obs/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `obs/config_test.go`:

```go
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
		name        string
		endpoint    string
		insecureEnv string
		wantHost    string
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./obs/ -run 'TestIsEnabled|TestOTLPEndpoint|TestOTLPHeaders' -v`
Expected: FAIL — `obs` package does not exist yet (`no required module provides package .../obs`).

- [ ] **Step 3: Write the implementation**

Create `obs/config.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./obs/ -run 'TestIsEnabled|TestOTLPEndpoint|TestOTLPHeaders' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add obs/config.go obs/config_test.go
git commit -m "feat(obs): add OTel enablement and OTLP endpoint parsing"
```

---

### Task 4: `obs/obs.go` + `obs/resource.go` — tracer/meter helpers and service resource

**Files:**
- Create: `obs/obs.go`, `obs/resource.go`
- Test: `obs/resource_test.go`

- [ ] **Step 1: Write the failing test**

Create `obs/resource_test.go`:

```go
package obs

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
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

var _ = attribute.String
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./obs/ -run TestBuildResource -v`
Expected: FAIL — package `obs` does not yet compile (`buildResource` undefined).

- [ ] **Step 3: Write the implementation**

Create `obs/obs.go`:

```go
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
```

Create `obs/resource.go`:

```go
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

	// Merge(env, base): env wins over base; base wins over resource.Default().
	return resource.Merge(envRes, resource.Merge(resource.Default(), base)), nil
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
```

Note: keep the `var _ = attribute.String` line out of the test — the import is only needed indirectly; if the compiler complains about the unused import, remove `attribute` from the test imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./obs/ -run TestBuildResource -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add obs/obs.go obs/resource.go obs/resource_test.go
git commit -m "feat(obs): add Tracer/Meter helpers and service resource builder"
```

---

### Task 5: `obs/init.go` — provider construction and shutdown

**Files:**
- Create: `obs/init.go`
- Test: `obs/init_test.go`

- [ ] **Step 1: Write the failing test**

Create `obs/init_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./obs/ -run 'TestInit' -v`
Expected: FAIL — `Init` undefined.

- [ ] **Step 3: Write the implementation**

Create `obs/init.go`:

```go
package obs

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
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

	traceClient, err := newTraceClient()
	if err != nil {
		return nil, err
	}
	traceExp, err := otlptrace.New(ctx, traceClient)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricClient, err := newMetricClient()
	if err != nil {
		return nil, err
	}
	metricExp, err := otlpmetric.New(ctx, metricClient)
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

func newTraceClient() (otlptracegrpc.Client, error) {
	host, insecure, err := otlpEndpoint()
	if err != nil {
		return nil, err
	}
	if host == "" {
		return otlptracegrpc.NewClient(), nil
	}
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(host)}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	headers, err := otlpHeaders()
	if err != nil {
		return nil, err
	}
	if len(headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(headers))
	}
	return otlptracegrpc.NewClient(opts...), nil
}

func newMetricClient() (otlpmetricgrpc.Client, error) {
	host, insecure, err := otlpEndpoint()
	if err != nil {
		return nil, err
	}
	if host == "" {
		return otlpmetricgrpc.NewClient(), nil
	}
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(host)}
	if insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	headers, err := otlpHeaders()
	if err != nil {
		return nil, err
	}
	if len(headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(headers))
	}
	return otlpmetricgrpc.NewClient(opts...), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./obs/ -run 'TestInit' -v`
Expected: PASS. (`TestInitEnabledInstallsProviders` may take ~1s flushing to the dead port; ignore.)

- [ ] **Step 5: Commit**

```bash
git add obs/init.go obs/init_test.go
git commit -m "feat(obs): add Init with OTLP trace and metric providers and shutdown"
```

---

### Task 6: `obs/imapops.go` — shared IMAP span+metric helper

**Files:**
- Create: `obs/imapops.go`

- [ ] **Step 1: Write the implementation**

Create `obs/imapops.go`:

```go
package obs

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	attrOperation   = attribute.Key("imap.operation")
	attrOutcome     = attribute.Key("outcome")
	attrMailbox     = attribute.Key("imap.mailbox")
	attrUIDCount    = attribute.Key("imap.uid_count")
	attrDestination = attribute.Key("imap.destination")
	attrExpunge     = attribute.Key("imap.expunge")
)

type imapInstruments struct {
	operations metric.Int64Counter
	duration   metric.Float64Histogram
	errors     metric.Int64Counter
}

func newIMAPInstruments(meter metric.Meter) imapInstruments {
	operations, _ := meter.Int64Counter(
		"postmanpat.imap.operations",
		metric.WithUnit("{op}"),
		metric.WithDescription("IMAP operations by type and outcome"),
	)
	duration, _ := meter.Float64Histogram(
		"postmanpat.imap.duration",
		metric.WithUnit("s"),
		metric.WithDescription("IMAP operation duration"),
	)
	errors, _ := meter.Int64Counter(
		"postmanpat.imap.errors",
		metric.WithUnit("{error}"),
		metric.WithDescription("IMAP operation errors"),
	)
	return imapInstruments{operations: operations, duration: duration, errors: errors}
}

// startIMAPOp starts an "imap.<operation>" span, optionally as a child of ctx.
func startIMAPOp(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	all := append(attrs, attrOperation.String(operation))
	return Tracer("github.com/aaronromeo/postmanpat/obs/imap").Start(ctx, "imap."+operation,
		trace.WithAttributes(all...))
}

// finishIMAPOp records the outcome, emits RED metrics, and ends the span.
// operation names the span; label is the short metric operation label.
func finishIMAPOp(ctx context.Context, inst imapInstruments, operation, label string, started time.Time, span trace.Span, err error) {
	outcome := "success"
	if err != nil {
		outcome = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.SetAttributes(attrOutcome.String(outcome))
	span.End()

	attrs := []attribute.KeyValue{attrOperation.String(label), attrOutcome.String(outcome)}
	inst.operations.Add(ctx, 1, metric.WithAttributes(attrs...))
	inst.duration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attrs...))
	if err != nil {
		inst.errors.Add(ctx, 1, metric.WithAttributes(attrOperation.String(label)))
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./obs/`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add obs/imapops.go
git commit -m "feat(obs): add shared IMAP span and RED metric helper"
```

---

### Task 7: `obs/cleanuprunner.go` — cleanup runner decorator

**Files:**
- Create: `obs/cleanuprunner.go`
- Test: `obs/cleanuprunner_test.go`

- [ ] **Step 1: Write the failing test**

Create `obs/cleanuprunner_test.go`:

```go
package obs

import (
	"context"
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	assert.Equal(t, "search", valueFor(t, spans[0], "imap.operation").AsString())
	assert.Equal(t, int64(1), valueFor(t, spans[0], "imap.uid_count").AsInt64())
	assert.Equal(t, "success", valueFor(t, spans[0], "outcome").AsString())

	var out metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &out))
	assert.Equal(t, int64(1), metricSum(t, out, "postmanpat.imap.operations"))
	assert.Equal(t, int64(1), metricCount(t, out, "postmanpat.imap.duration"))
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
```

Create `obs/testhelpers_test.go` (used by this and later obs tests):

```go
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
			h, ok := m.Data.(metricdata.Histogram)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./obs/ -run TestWrapCleanupRunner -v`
Expected: FAIL — `WrapCleanupRunner` undefined.

- [ ] **Step 3: Write the implementation**

Create `obs/cleanuprunner.go`:

```go
package obs

import (
	"context"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/serverrunner"
)

type cleanupRunnerWrapper struct {
	inner serverrunner.ServerRunner
	inst  imapInstruments
}

// WrapCleanupRunner returns an instrumented serverrunner.ServerRunner. Each
// interface method runs inside an imap.<op> span and emits postmanpat.imap.*
// RED metrics.
func WrapCleanupRunner(inner serverrunner.ServerRunner) serverrunner.ServerRunner {
	return &cleanupRunnerWrapper{
		inner: inner,
		inst:  newIMAPInstruments(Meter("github.com/aaronromeo/postmanpat/obs/cleanuprunner")),
	}
}

func (w *cleanupRunnerWrapper) Connect() error {
	ctx, span := startIMAPOp(context.Background(), "connect")
	started := time.Now()
	err := w.inner.Connect()
	finishIMAPOp(ctx, w.inst, "connect", "connect", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) Close() error {
	ctx, span := startIMAPOp(context.Background(), "close")
	started := time.Now()
	err := w.inner.Close()
	finishIMAPOp(ctx, w.inst, "close", "close", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) SearchByServerMatchers(ctx context.Context, matchers appconfig.ServerMatchers) (map[string][]uint32, error) {
	started := time.Now()
	mailbox := ""
	if len(matchers.Folders) > 0 {
		mailbox = matchers.Folders[0]
	}
	spanCtx, span := startIMAPOp(ctx, "search_by_server_matchers", attrMailbox.String(mailbox))
	result, err := w.inner.SearchByServerMatchers(spanCtx, matchers)
	if err == nil {
		span.SetAttributes(attrUIDCount.Int(len(result[mailbox])))
	}
	finishIMAPOp(spanCtx, w.inst, "search_by_server_matchers", "search", started, span, err)
	return result, err
}

func (w *cleanupRunnerWrapper) MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "move_by_mailbox",
		attrDestination.String(destination),
		attrUIDCount.Int(uidCount(uidsByMailbox)),
	)
	err := w.inner.MoveByMailbox(spanCtx, uidsByMailbox, destination)
	finishIMAPOp(spanCtx, w.inst, "move_by_mailbox", "move", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "delete_by_mailbox",
		attrExpunge.Bool(expunge),
		attrUIDCount.Int(uidCount(uidsByMailbox)),
	)
	err := w.inner.DeleteByMailbox(spanCtx, uidsByMailbox, expunge)
	finishIMAPOp(spanCtx, w.inst, "delete_by_mailbox", "delete", started, span, err)
	return err
}

func (w *cleanupRunnerWrapper) FetchSenderDataByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32) (map[string][]imap.MailData, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "fetch_sender_data", attrUIDCount.Int(uidCount(uidsByMailbox)))
	result, err := w.inner.FetchSenderDataByMailbox(spanCtx, uidsByMailbox)
	finishIMAPOp(spanCtx, w.inst, "fetch_sender_data", "fetch", started, span, err)
	return result, err
}

func uidCount(uidsByMailbox map[string][]uint32) int {
	total := 0
	for _, uids := range uidsByMailbox {
		total += len(uids)
	}
	return total
}
```

> Note: `FetchSenderDataByMailbox` is added to the `serverrunner.ServerRunner` interface in Task 11. Until then this file won't compile; that is fine — complete Task 11 before running the full build, or implement Task 11 immediately after this step.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./obs/ -run TestWrapCleanupRunner -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add obs/cleanuprunner.go obs/cleanuprunner_test.go obs/testhelpers_test.go
git commit -m "feat(obs): add cleanup runner decorator with IMAP spans and RED metrics"
```

---

### Task 8: `obs/watchrunner.go` — watch runner decorator

**Files:**
- Create: `obs/watchrunner.go`
- Test: `obs/watchrunner_test.go`

- [ ] **Step 1: Write the failing test**

Create `obs/watchrunner_test.go`:

```go
package obs

import (
	"context"
	"testing"

	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type fakeWatchRunner struct{}

func (f *fakeWatchRunner) Connect() error                                   { return nil }
func (f *fakeWatchRunner) Close() error                                     { return nil }
func (f *fakeWatchRunner) Idle() (*giimapclient.IdleCommand, error)         { return nil, nil }
func (f *fakeWatchRunner) SelectMailbox(ctx context.Context, m string) (*giimap.SelectData, error) {
	return nil, nil
}
func (f *fakeWatchRunner) FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error) {
	return nil, nil
}
func (f *fakeWatchRunner) SearchUIDsNewerThan(ctx context.Context, last uint32) ([]uint32, error) {
	return nil, nil
}
func (f *fakeWatchRunner) MoveUIDs(ctx context.Context, uids []uint32, dest string) error { return nil }
func (f *fakeWatchRunner) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	return nil
}

func TestWrapWatchRunnerSelectEmitsSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	wrapped := WrapWatchRunner(&fakeWatchRunner{})
	_, err := wrapped.SelectMailbox(context.Background(), "INBOX")
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "imap.select", spans[0].Name())
	assert.Equal(t, "select", valueFor(t, spans[0], "imap.operation").AsString())
	assert.Equal(t, attribute.StringValue("INBOX"), valueFor(t, spans[0], "imap.mailbox"))
	assert.Equal(t, "success", valueFor(t, spans[0], "outcome").AsString())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./obs/ -run TestWrapWatchRunner -v`
Expected: FAIL — `WrapWatchRunner` undefined.

- [ ] **Step 3: Write the implementation**

Create `obs/watchrunner.go`:

```go
package obs

import (
	"context"
	"time"

	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/watchrunner"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
)

type watchRunnerWrapper struct {
	inner watchrunner.WatchRunner
	inst  imapInstruments
}

// WrapWatchRunner returns an instrumented watchrunner.WatchRunner. Each
// interface method runs inside an imap.<op> span and emits the shared
// postmanpat.imap.* RED metrics.
func WrapWatchRunner(inner watchrunner.WatchRunner) watchrunner.WatchRunner {
	return &watchRunnerWrapper{
		inner: inner,
		inst:  newIMAPInstruments(Meter("github.com/aaronromeo/postmanpat/obs/watchrunner")),
	}
}

func (w *watchRunnerWrapper) Connect() error {
	ctx, span := startIMAPOp(context.Background(), "connect")
	started := time.Now()
	err := w.inner.Connect()
	finishIMAPOp(ctx, w.inst, "connect", "connect", started, span, err)
	return err
}

func (w *watchRunnerWrapper) Close() error {
	ctx, span := startIMAPOp(context.Background(), "close")
	started := time.Now()
	err := w.inner.Close()
	finishIMAPOp(ctx, w.inst, "close", "close", started, span, err)
	return err
}

func (w *watchRunnerWrapper) Idle() (*giimapclient.IdleCommand, error) {
	ctx, span := startIMAPOp(context.Background(), "idle.start")
	started := time.Now()
	result, err := w.inner.Idle()
	finishIMAPOp(ctx, w.inst, "idle.start", "idle_start", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) SelectMailbox(ctx context.Context, mailbox string) (*giimap.SelectData, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "select", attrMailbox.String(mailbox))
	result, err := w.inner.SelectMailbox(spanCtx, mailbox)
	finishIMAPOp(spanCtx, w.inst, "select", "select", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "fetch_sender_data", attrUIDCount.Int(len(uids)))
	result, err := w.inner.FetchSenderData(spanCtx, uids)
	finishIMAPOp(spanCtx, w.inst, "fetch_sender_data", "fetch", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) SearchUIDsNewerThan(ctx context.Context, lastUID uint32) ([]uint32, error) {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "search_newer_than")
	result, err := w.inner.SearchUIDsNewerThan(spanCtx, lastUID)
	if err == nil {
		span.SetAttributes(attrUIDCount.Int(len(result)))
	}
	finishIMAPOp(spanCtx, w.inst, "search_newer_than", "search", started, span, err)
	return result, err
}

func (w *watchRunnerWrapper) MoveUIDs(ctx context.Context, uids []uint32, destination string) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "move", attrDestination.String(destination), attrUIDCount.Int(len(uids)))
	err := w.inner.MoveUIDs(spanCtx, uids, destination)
	finishIMAPOp(spanCtx, w.inst, "move", "move", started, span, err)
	return err
}

func (w *watchRunnerWrapper) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	started := time.Now()
	spanCtx, span := startIMAPOp(ctx, "delete", attrExpunge.Bool(expunge), attrUIDCount.Int(len(uids)))
	err := w.inner.DeleteUIDs(spanCtx, uids, expunge)
	finishIMAPOp(spanCtx, w.inst, "delete", "delete", started, span, err)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./obs/ -run TestWrapWatchRunner -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add obs/watchrunner.go obs/watchrunner_test.go
git commit -m "feat(obs): add watch runner decorator with IMAP spans and RED metrics"
```

---

## Phase 2 — CLI wiring

### Task 9: `main.go` calls `obs.Init`, supports signals, flushes on exit

**Files:**
- Modify: `cmd/postmanpat/main.go`, `cli/root.go`

- [ ] **Step 1: Add context-aware execute entrypoint**

In `cli/root.go`, replace `Execute` with:

```go
// ExecuteWithContext runs the root command with the given context and returns
// the process exit code.
func ExecuteWithContext(ctx context.Context) int {
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Execute runs the root command and exits the process.
func Execute() {
	os.Exit(ExecuteWithContext(context.Background()))
}
```

Add `"context"` to the `cli/root.go` imports.

- [ ] **Step 2: Run existing tests to confirm no regression**

Run: `go test ./cli/ ./cmd/...`
Expected: PASS.

- [ ] **Step 3: Rewrite `main.go`**

Replace `cmd/postmanpat/main.go` with:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aaronromeo/postmanpat/cli"
	"github.com/aaronromeo/postmanpat/obs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := obs.Init(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "observability init failed:", err)
		os.Exit(1)
	}

	code := cli.ExecuteWithContext(ctx)

	// Use a fresh context so a signal-cancelled ctx cannot abort the flush.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		fmt.Fprintln(os.Stderr, "observability shutdown failed:", err)
		code = 1
	}
	os.Exit(code)
}
```

- [ ] **Step 4: Verify build and tests**

Run: `go build ./... && go test ./cli/ ./cmd/...`
Expected: both exit 0.

- [ ] **Step 5: Commit**

```bash
git add cmd/postmanpat/main.go cli/root.go
git commit -m "feat: initialize OTel in main, handle signals, flush telemetry on exit"
```

---

## Phase 3 — Domain instrumentation

### Task 10: Add `MessageID` to `MailData`

**Files:**
- Modify: `imap/internal/maildata/types.go`, `imap/internal/selectors/manager.go`

- [ ] **Step 1: Add the field**

In `imap/internal/maildata/types.go`, add after `UID uint32`:

```go
	MessageID              string
```

- [ ] **Step 2: Fetch and populate Message-ID**

In `imap/internal/selectors/manager.go`:
1. In `FetchSenderData`, add `"Message-ID"` to the `HeaderFields` slice (line ~121).
2. In the `data := foo.MailData{...}` literal, add:

```go
		MessageID:            headerText(header, "Message-ID"),
```

- [ ] **Step 3: Verify with a focused test**

Run: `go test ./imap/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add imap/internal/maildata/types.go imap/internal/selectors/manager.go
git commit -m "feat(imap): expose RFC 5322 Message-ID in MailData for trace attributes"
```

---

### Task 11: Extend `serverrunner.ServerRunner` with `FetchSenderDataByMailbox`

**Files:**
- Modify: `serverrunner/runner.go`

- [ ] **Step 1: Add the interface method**

Replace `serverrunner/runner.go` with:

```go
package serverrunner

import (
	"context"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
)

type ServerRunner interface {
	Connect() error
	Close() error
	SearchByServerMatchers(ctx context.Context, matchers appconfig.ServerMatchers) (map[string][]uint32, error)
	MoveByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, destination string) error
	DeleteByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32, expunge bool) error
	FetchSenderDataByMailbox(ctx context.Context, uidsByMailbox map[string][]uint32) (map[string][]imap.MailData, error)
}

func New(opts ...imap.Option) *imap.Client {
	return imap.NewServer(opts...)
}
```

- [ ] **Step 2: Verify compile and obs tests now pass**

Run: `go build ./... && go test ./obs/ ./serverrunner/ ./ftest/`
Expected: PASS. (`*imap.Client` already satisfies the new method; `WrapCleanupRunner` now compiles.)

- [ ] **Step 3: Commit**

```bash
git add serverrunner/runner.go
git commit -m "feat(serverrunner): add FetchSenderDataByMailbox to the runner interface"
```

---

### Task 12: Instrument `cli/cleanup.go` — invocation, rule, action spans + per-message events

**Files:**
- Modify: `cli/cleanup.go`

- [ ] **Step 1: Add imports**

Add to `cli/cleanup.go`:

```go
	"time"

	"github.com/aaronromeo/postmanpat/obs"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
```

- [ ] **Step 2: Wrap the client and emit domain spans**

Make these changes to the `RunE` body:

1. After building `client`, wrap it:

```go
		var client serverrunner.ServerRunner = serverrunner.New(
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
		)
		client = obs.WrapCleanupRunner(client)
```

2. After the `dryRun` flag read, add instrumentation setup and the root span:

```go
		tracer := obs.Tracer("github.com/aaronromeo/postmanpat/cli")
		meter := obs.Meter("github.com/aaronromeo/postmanpat/cleanuprunner")
		invocations, _ := meter.Int64Counter("postmanpat.cleanup.invocations", metric.WithUnit("{run}"))
		runDuration, _ := meter.Float64Histogram("postmanpat.cleanup.duration", metric.WithUnit("s"))
		ruleMatches, _ := meter.Int64Counter("postmanpat.cleanup.rule.matches", metric.WithUnit("{message}"))
		actionMessages, _ := meter.Int64Counter("postmanpat.cleanup.action.messages", metric.WithUnit("{message}"))
		actionErrors, _ := meter.Int64Counter("postmanpat.cleanup.action.errors", metric.WithUnit("{error}"))

		invCtx, invSpan := tracer.Start(ctx, "cleanup.invocation",
			trace.WithAttributes(
				attribute.String("postmanpat.command", "cleanup"),
				attribute.Bool("postmanpat.dry_run", dryRun),
				attribute.String("postmanpat.config_path", cfgPath),
				attribute.Int("postmanpat.rules.count", len(cfg.Rules)),
			))
		runStarted := time.Now()
		ctx = invCtx
```

3. Replace the `if err := client.Connect(); err != nil { return err }` block with:

```go
		if err := client.Connect(); err != nil {
			invSpan.RecordError(err)
			invSpan.SetStatus(codes.Error, err.Error())
			invSpan.End()
			invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
			runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
			return err
		}
		defer client.Close()
```

4. Replace the rule loop (`for _, rule := range cfg.Rules { ... }`) with the instrumented version:

```go
		rulesMatched := 0
		messagesMatched := 0
		for _, rule := range cfg.Rules {
			mailbox := rule.Server.Folders[0]
			ruleCtx, ruleSpan := tracer.Start(ctx, "cleanup.rule",
				trace.WithAttributes(
					attribute.String("rule.name", rule.Name),
					attribute.String("rule.mailbox", mailbox),
					attribute.StringSlice("rule.actions", actionNames(rule)),
				))

			matched, err := client.SearchByServerMatchers(ruleCtx, *rule.Server)
			if err != nil {
				ruleSpan.RecordError(err)
				ruleSpan.SetStatus(codes.Error, err.Error())
				ruleSpan.End()
				invSpan.RecordError(err)
				invSpan.SetStatus(codes.Error, err.Error())
				invSpan.End()
				invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
				runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "error"), attribute.Bool("postmanpat.dry_run", dryRun)))
				return err
			}
			uids := matched[mailbox]
			if len(uids) > 0 {
				rulesMatched++
			}
			messagesMatched += len(uids)
			ruleSpan.SetAttributes(attribute.Int("rule.matched_count", len(uids)))
			ruleMatches.Add(ruleCtx, int64(len(uids)), metric.WithAttributes(
				attribute.String("rule.name", rule.Name),
				attribute.String("mailbox", mailbox),
			))

			logger.Info("rule matched", "rule", rule.Name, "mailbox", mailbox, "messages", len(uids))
			if len(uids) > 0 {
				if err := announcerService.Do("Cleanup", rule.Name, mailbox, len(uids)); err != nil {
					logger.Error("reporting failed", "rule", rule.Name, "mailbox", mailbox, "error", err)
				}
			}

			for _, action := range rule.Actions {
				actionCtx, actionSpan := tracer.Start(ruleCtx, "cleanup.action",
					trace.WithAttributes(
						attribute.String("action.type", string(action.Type)),
						attribute.String("action.destination", action.Destination),
						attribute.Bool("action.dry_run", dryRun),
						attribute.Int("action.uid_count", len(uids)),
					))

				if len(uids) > 0 {
					dataByMailbox, ferr := client.FetchSenderDataByMailbox(actionCtx, matched)
					if ferr != nil {
						actionSpan.RecordError(ferr)
						actionSpan.SetStatus(codes.Error, ferr.Error())
						actionSpan.End()
						actionErrors.Add(actionCtx, 1, metric.WithAttributes(
							attribute.String("action.type", string(action.Type)),
							attribute.String("rule.name", rule.Name),
						))
						return ferr
					}
					for _, md := range dataByMailbox[mailbox] {
						actionSpan.AddEvent("action.message_identified", trace.WithAttributes(
							attribute.Int64("imap.uid", int64(md.UID)),
							attribute.String("email.message_id", md.MessageID),
							attribute.StringSlice("email.from", md.From),
							attribute.String("email.subject", md.SubjectRaw),
							attribute.String("email.internal_date", md.MessageDate.UTC().Format(time.RFC3339)),
						))
					}
					actionSpan.AddEvent("action.applied",
						trace.WithAttributes(
							attribute.Int("action.uid_count", len(uids)),
							attribute.Bool("action.dry_run", dryRun),
						))
				}

				switch action.Type {
				case appconfig.DELETE:
					if dryRun {
						logger.Info("dry run delete", "rule", rule.Name, "messages", len(uids))
						break
					}
					expungeAfterDelete := true
					if action.ExpungeAfterDelete != nil {
						expungeAfterDelete = *action.ExpungeAfterDelete
					}
					if err := client.DeleteByMailbox(actionCtx, matched, expungeAfterDelete); err != nil {
						actionSpan.RecordError(err)
						actionSpan.SetStatus(codes.Error, err.Error())
						actionSpan.End()
						actionErrors.Add(actionCtx, 1, metric.WithAttributes(
							attribute.String("action.type", string(action.Type)),
							attribute.String("rule.name", rule.Name),
						))
						return err
					}
				case appconfig.MOVE:
					if strings.TrimSpace(action.Destination) == "" {
						return fmt.Errorf("Action move missing destination: %s", rule.Name)
					}
					if dryRun {
						logger.Info("dry run move", "rule", rule.Name, "messages", len(uids))
						break
					}
					if err := client.MoveByMailbox(actionCtx, matched, strings.TrimSpace(action.Destination)); err != nil {
						actionSpan.RecordError(err)
						actionSpan.SetStatus(codes.Error, err.Error())
						actionSpan.End()
						actionErrors.Add(actionCtx, 1, metric.WithAttributes(
							attribute.String("action.type", string(action.Type)),
							attribute.String("rule.name", rule.Name),
						))
						return err
					}
				default:
					return fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
				}

				actionMessages.Add(actionCtx, int64(len(uids)), metric.WithAttributes(
					attribute.String("action.type", string(action.Type)),
					attribute.String("rule.name", rule.Name),
					attribute.String("destination", action.Destination),
					attribute.Bool("postmanpat.dry_run", dryRun),
				))
				actionSpan.End()
			}
			ruleSpan.End()
		}

		invSpan.SetAttributes(
			attribute.Int("postmanpat.rules.matched", rulesMatched),
			attribute.Int("postmanpat.messages.matched", messagesMatched),
		)
		invSpan.End()
		invocations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "success"), attribute.Bool("postmanpat.dry_run", dryRun)))
		runDuration.Record(ctx, time.Since(runStarted).Seconds(), metric.WithAttributes(attribute.String("outcome", "success"), attribute.Bool("postmanpat.dry_run", dryRun)))
		return nil
```

5. Add the helper function at the end of the file:

```go
func actionNames(rule appconfig.Rule) []string {
	names := make([]string, len(rule.Actions))
	for i, a := range rule.Actions {
		names[i] = string(a.Type)
	}
	return names
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./... && go vet ./cli/`
Expected: both exit 0.

- [ ] **Step 4: Run existing cli tests**

Run: `go test ./cli/`
Expected: PASS (`TestCleanupRejectsClientMatchers` still passes — it errors before the instrumentation).

- [ ] **Step 5: Commit**

```bash
git add cli/cleanup.go
git commit -m "feat(cleanup): emit invocation, rule, and action spans with per-message events"
```

---

### Task 13: Cleanup trace integration test (in-memory IMAP + in-memory OTel)

**Files:**
- Create: `cli/cleanup_trace_test.go`

- [ ] **Step 1: Write the test**

Create `cli/cleanup_trace_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aaronromeo/postmanpat/ftest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestCleanupEmitsInvocationTrace(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	addr, _, cleanupServer := ftest.SetupIMAPServer(t, nil, []string{"Archive"}, nil)
	t.Cleanup(cleanupServer)
	host, port, err := splitHostPort(addr)
	require.NoError(t, err)

	t.Setenv("POSTMANPAT_IMAP_HOST", host)
	t.Setenv("POSTMANPAT_IMAP_PORT", port)
	t.Setenv("POSTMANPAT_IMAP_USER", ftest.DefaultUser)
	t.Setenv("POSTMANPAT_IMAP_PASS", ftest.DefaultPass)
	t.Setenv("POSTMANPAT_S3_ENDPOINT", "https://nyc3.digitaloceanspaces.com")
	t.Setenv("POSTMANPAT_S3_REGION", "nyc3")
	t.Setenv("POSTMANPAT_S3_BUCKET", "bucket")
	t.Setenv("POSTMANPAT_S3_KEY", "key")
	t.Setenv("POSTMANPAT_S3_SECRET", "secret")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
rules:
  - name: "NewsRule"
    server:
      folders: ["INBOX"]
      sender_substring: ["example.com"]
    actions:
      - type: move
        destination: Archive
`), 0o600))

	rootCmd.SetArgs([]string{"cleanup", "--config", path})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	require.NoError(t, rootCmd.Execute())

	spans := rec.Ended()
	var invocation, ruleSpan, actionSpan, searchSpan *sdktrace.ReadOnlySpan
	for i := range spans {
		switch spans[i].Name() {
		case "cleanup.invocation":
			invocation = &spans[i]
		case "cleanup.rule":
			ruleSpan = &spans[i]
		case "cleanup.action":
			actionSpan = &spans[i]
		case "imap.search_by_server_matchers":
			searchSpan = &spans[i]
		}
	}
	require.NotNil(t, invocation, "missing cleanup.invocation span")
	require.NotNil(t, ruleSpan, "missing cleanup.rule span")
	require.NotNil(t, actionSpan, "missing cleanup.action span")
	require.NotNil(t, searchSpan, "missing imap.search_by_server_matchers span")

	assert.Equal(t, invocation.SpanContext().SpanID(), ruleSpan.Parent().SpanID(), "rule should be child of invocation")
	assert.Equal(t, ruleSpan.SpanContext().SpanID(), searchSpan.Parent().SpanID(), "search should be child of rule")
	assert.Equal(t, ruleSpan.SpanContext().SpanID(), actionSpan.Parent().SpanID(), "action should be child of rule")
	assert.Equal(t, "NewsRule", attrString(t, ruleSpan, "rule.name"))
	assert.Equal(t, int64(1), attrInt(t, ruleSpan, "rule.matched_count"))

	var sawMessageEvent bool
	for _, ev := range actionSpan.Events() {
		if ev.Name != "action.message_identified" {
			continue
		}
		for _, kv := range ev.Attributes {
			if kv.Key == attribute.Key("email.subject") && kv.Value.AsString() == "Hello" {
				sawMessageEvent = true
			}
		}
	}
	assert.True(t, sawMessageEvent, "expected action.message_identified event with subject Hello")
}

func attrString(t *testing.T, span *sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

func attrInt(t *testing.T, span *sdktrace.ReadOnlySpan, key string) int64 {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsInt64()
		}
	}
	return 0
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./cli/ -run TestCleanupEmitsInvocationTrace -v`
Expected: PASS. The rule matches the ftest "News" message (`example.com`, subject "Hello"), moves it to `Archive`, and emits the full span tree with a `message_identified` event.

- [ ] **Step 3: Commit**

```bash
git add cli/cleanup_trace_test.go
git commit -m "test(cleanup): assert invocation/rule/action trace shape against in-memory IMAP"
```

---

### Task 14: Refactor `watchrunner` to free functions taking the runner interface

**Files:**
- Modify: `watchrunner/runner.go`, `cli/watch.go`, `ftest/watchrunner_integration_test.go`

- [ ] **Step 1: Convert `ProcessUIDs` and `Reconnect` to free functions**

In `watchrunner/runner.go`:
1. Change the receiver on `ProcessUIDs` and `Reconnect` from `func (c *Client)` to plain functions whose first parameter is `c WatchRunner`.
2. Inside `ProcessUIDs`, change the `c.FetchSenderData(deps.Ctx, uids)` call (unchanged — `c` is now the param) and the `applyActions(c, deps, rule, message.UID)` call (now `applyActions(deps.Ctx, c, deps, rule, message.UID)`).
3. Inside `Reconnect`, call `ProcessUIDs(c, deps, state, uids)`.

The new signatures:

```go
func ProcessUIDs(c WatchRunner, deps Deps, state *State, uids []uint32) error
func Reconnect(c WatchRunner, deps Deps, state *State, mailbox string) error
func applyActions(ctx context.Context, client WatchRunner, deps Deps, rule appconfig.Rule, uid uint32) error
```

`applyActions` gains a leading `ctx context.Context` parameter and uses `client.DeleteUIDs(ctx, ...)` / `client.MoveUIDs(ctx, ...)` with that ctx. Its body otherwise keeps the existing action logic.

- [ ] **Step 2: Update callers**

In `cli/watch.go`:
- `client.ProcessUIDs(deps, state, uids)` → `watchrunner.ProcessUIDs(client, deps, state, uids)`
- `client.Reconnect(deps, state, defaultMailbox)` → `watchrunner.Reconnect(client, deps, state, defaultMailbox)`

In `ftest/watchrunner_integration_test.go`, each `client.ProcessUIDs(deps, state, ...)` → `watchrunner.ProcessUIDs(client, deps, state, ...)`.

- [ ] **Step 3: Verify**

Run: `go build ./... && go test ./watchrunner/ ./ftest/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add watchrunner/runner.go cli/watch.go ftest/watchrunner_integration_test.go
git commit -m "refactor(watchrunner): take the runner interface so decorator spans thread through"
```

---

### Task 15: Instrument `watchrunner` — `watch.message` + `watch.rule_evaluated` + `watch.action`

**Files:**
- Modify: `watchrunner/runner.go`
- Test: `watchrunner/trace_test.go`

- [ ] **Step 1: Add OTel imports and instrument helpers**

Add to `watchrunner/runner.go` imports:

```go
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
```

Add package-level lazy instruments:

```go
type watchInstruments struct {
	messages       metric.Int64Counter
	ruleMatches    metric.Int64Counter
	actionMessages metric.Int64Counter
}

var watchInstrumentsOnce = sync.OnceValue(func() watchInstruments {
	meter := otel.Meter("github.com/aaronromeo/postmanpat/watchrunner")
	messages, _ := meter.Int64Counter("postmanpat.watch.messages.processed", metric.WithUnit("{message}"))
	ruleMatches, _ := meter.Int64Counter("postmanpat.watch.rule.matches", metric.WithUnit("{message}"))
	actionMessages, _ := meter.Int64Counter("postmanpat.watch.action.messages", metric.WithUnit("{message}"))
	return watchInstruments{messages: messages, ruleMatches: ruleMatches, actionMessages: actionMessages}
})
```

- [ ] **Step 2: Write the failing unit test**

Create `watchrunner/trace_test.go`:

```go
package watchrunner

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	giimap "github.com/emersion/go-imap/v2"
	giimapclient "github.com/emersion/go-imap/v2/imapclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type fakeRunner struct {
	data  []imap.MailData
	moved []uint32
}

func (f *fakeRunner) Connect() error                                   { return nil }
func (f *fakeRunner) Close() error                                     { return nil }
func (f *fakeRunner) Idle() (*giimapclient.IdleCommand, error)         { return nil, nil }
func (f *fakeRunner) SelectMailbox(ctx context.Context, m string) (*giimap.SelectData, error) {
	return nil, nil
}
func (f *fakeRunner) FetchSenderData(ctx context.Context, uids []uint32) ([]imap.MailData, error) {
	return f.data, nil
}
func (f *fakeRunner) SearchUIDsNewerThan(ctx context.Context, last uint32) ([]uint32, error) {
	return nil, nil
}
func (f *fakeRunner) MoveUIDs(ctx context.Context, uids []uint32, dest string) error {
	f.moved = append(f.moved, uids...)
	return nil
}
func (f *fakeRunner) DeleteUIDs(ctx context.Context, uids []uint32, expunge bool) error {
	return nil
}

func TestProcessUIDsEmitsRuleEvaluationTrace(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	ruleMatch := appconfig.Rule{
		Name:   "MatchRule",
		Client: &appconfig.ClientMatchers{SenderRegex: []string{`example\.com`}},
		Actions: []appconfig.Action{{Type: appconfig.MOVE, Destination: "Archive"}},
	}
	ruleNoMatch := appconfig.Rule{
		Name:   "NoMatchRule",
		Client: &appconfig.ClientMatchers{SenderRegex: []string{`nope\.com`}},
	}
	deps := Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{ruleMatch, ruleNoMatch},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	runner := &fakeRunner{data: []imap.MailData{{
		UID:           1,
		From:          []string{"news@example.com"},
		SenderDomains: []string{"example.com"},
		SubjectRaw:    "Hello",
		MessageDate:   time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
	}}}
	state := &State{}

	require.NoError(t, ProcessUIDs(runner, deps, state, []uint32{1}))
	assert.Equal(t, []uint32{1}, runner.moved)

	spans := rec.Ended()
	var msgSpan, actionSpan *sdktrace.ReadOnlySpan
	for i := range spans {
		switch spans[i].Name() {
		case "watch.message":
			msgSpan = &spans[i]
		case "watch.action":
			actionSpan = &spans[i]
		}
	}
	require.NotNil(t, msgSpan, "missing watch.message span")
	require.NotNil(t, actionSpan, "missing watch.action span")

	assert.Equal(t, int64(1), attrIntFromReadOnly(t, msgSpan, "imap.uid"))
	assert.Equal(t, "Hello", attrStringFromReadOnly(t, msgSpan, "email.subject"))

	var evNames []string
	var ruleNames []string
	var matched []bool
	for _, ev := range msgSpan.Events() {
		if ev.Name != "watch.rule_evaluated" {
			continue
		}
		evNames = append(evNames, ev.Name)
		for _, kv := range ev.Attributes {
			if kv.Key == attribute.Key("rule.name") {
				ruleNames = append(ruleNames, kv.Value.AsString())
			}
			if kv.Key == attribute.Key("matched") {
				matched = append(matched, kv.Value.AsBool())
			}
		}
	}
	assert.Len(t, evNames, 2, "one rule_evaluated event per rule")
	assert.Equal(t, []string{"MatchRule", "NoMatchRule"}, ruleNames)
	assert.Equal(t, []bool{true, false}, matched)

	assert.Equal(t, msgSpan.SpanContext().SpanID(), actionSpan.Parent().SpanID(), "action should be child of message span")
}

func attrStringFromReadOnly(t *testing.T, span *sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

func attrIntFromReadOnly(t *testing.T, span *sdktrace.ReadOnlySpan, key string) int64 {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.AsInt64()
		}
	}
	return 0
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./watchrunner/ -run TestProcessUIDsEmitsRuleEvaluationTrace -v`
Expected: FAIL — no `watch.message` span is emitted yet.

- [ ] **Step 4: Instrument `ProcessUIDs` and `applyActions`**

In `watchrunner/runner.go`, replace the body of `ProcessUIDs` between the `data, err := c.FetchSenderData(...)` call and the `state.LastUID = maxUID(...)` line:

```go
	deps.Log.Debug("fetched messages for processing", "messages", len(data))
	tracer := otel.Tracer("github.com/aaronromeo/postmanpat/watchrunner")
	for _, message := range data {
		messageCtx, msgSpan := tracer.Start(deps.Ctx, "watch.message",
			trace.WithAttributes(
				attribute.Int64("imap.uid", int64(message.UID)),
				attribute.String("email.message_id", message.MessageID),
				attribute.StringSlice("email.from", message.From),
				attribute.String("email.subject", message.SubjectRaw),
				attribute.String("email.internal_date", message.MessageDate.UTC().Format(time.RFC3339)),
			))
		watchInstrumentsOnce().messages.Add(messageCtx, 1)

		matchedAny := false
		for _, rule := range deps.Rules {
			ok, err := (matchers.ClientMessage{
				ListID:           message.ListID,
				SenderDomains:    message.SenderDomains,
				ReplyToDomains:   message.ReplyToDomains,
				SubjectRaw:       message.SubjectRaw,
				Recipients:       message.Recipients,
				RecipientTags:    message.RecipientTags,
				Body:             message.Body,
				Cc:               message.Cc,
				ReturnPathDomain: message.ReturnPathDomain,
				ListUnsubscribe:  message.ListUnsubscribe,
			}).Match(rule.Client)
			if err != nil {
				msgSpan.RecordError(err)
				msgSpan.SetStatus(codes.Error, err.Error())
				msgSpan.End()
				return err
			}
			msgSpan.AddEvent("watch.rule_evaluated",
				trace.WithAttributes(
					attribute.String("rule.name", rule.Name),
					attribute.Bool("matched", ok),
				))
			if ok {
				matchedAny = true
				watchInstrumentsOnce().ruleMatches.Add(messageCtx, 1,
					metric.WithAttributes(attribute.String("rule.name", rule.Name)))
				deps.Log.InfoContext(messageCtx, "rule matched", "rule", rule.Name, "list_id", message.ListID)
				if deps.Announce != nil {
					deps.Announce(rule.Name)
				}
				if err := applyActions(messageCtx, c, deps, rule, message.UID); err != nil {
					msgSpan.RecordError(err)
					msgSpan.SetStatus(codes.Error, err.Error())
					msgSpan.End()
					return err
				}
			}
		}
		if !matchedAny {
			deps.Log.InfoContext(messageCtx, "no rule matched")
			if deps.Announce != nil {
				deps.Announce("")
			}
		}
		msgSpan.End()
	}
```

Replace `applyActions` with:

```go
func applyActions(ctx context.Context, client WatchRunner, deps Deps, rule appconfig.Rule, uid uint32) error {
	if uid == 0 {
		return nil
	}
	tracer := otel.Tracer("github.com/aaronromeo/postmanpat/watchrunner")
	for _, action := range rule.Actions {
		actionCtx, span := tracer.Start(ctx, "watch.action",
			trace.WithAttributes(
				attribute.String("rule.name", rule.Name),
				attribute.String("action.type", string(action.Type)),
				attribute.String("action.destination", action.Destination),
			))
		var err error
		switch action.Type {
		case appconfig.DELETE:
			expungeAfterDelete := true
			if action.ExpungeAfterDelete != nil {
				expungeAfterDelete = *action.ExpungeAfterDelete
			}
			err = client.DeleteUIDs(actionCtx, []uint32{uid}, expungeAfterDelete)
		case appconfig.MOVE:
			destination := strings.TrimSpace(action.Destination)
			if destination == "" {
				err = fmt.Errorf("Action move missing destination for rule %q", rule.Name)
			} else {
				err = client.MoveUIDs(actionCtx, []uint32{uid}, destination)
			}
		default:
			err = fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return err
		}
		watchInstrumentsOnce().actionMessages.Add(actionCtx, 1, metric.WithAttributes(
			attribute.String("action.type", string(action.Type)),
			attribute.String("rule.name", rule.Name),
			attribute.String("destination", action.Destination),
		))
		span.End()
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./watchrunner/ -run TestProcessUIDsEmitsRuleEvaluationTrace -v`
Expected: PASS.

- [ ] **Step 6: Run full package test suite**

Run: `go test ./watchrunner/ ./ftest/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add watchrunner/runner.go watchrunner/trace_test.go
git commit -m "feat(watchrunner): emit watch.message span with per-rule evaluation events"
```

---

### Task 16: Instrument `cli/watch.go` — connect / reconnect / reload / cycle spans

**Files:**
- Modify: `cli/watch.go`

- [ ] **Step 1: Add imports**

Add to `cli/watch.go`:

```go
	"crypto/rand"
	"encoding/hex"

	"github.com/aaronromeo/postmanpat/obs"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
```

(`"time"` is already imported.)

- [ ] **Step 2: Wrap the client and add session id helper**

1. After `client := watchrunner.New(...)`, wrap it:

```go
		var client watchrunner.WatchRunner = watchrunner.New(
			imap.WithAddr(
				fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port),
			),
			imap.WithCreds(imapEnv.User, imapEnv.Pass),
			imap.WithTLSConfig(tlsConfig),
			imap.WithUnilateralDataHandler(handler),
		)
		client = obs.WrapWatchRunner(client)
```

2. Add a helper at the end of the file:

```go
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
```

- [ ] **Step 3: Instrument startup connect + test-mode branch**

Replace the logger construction through the `SelectMailbox` call so the sequence is:

```go
		out := cmd.OutOrStdout()
		logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: logLevel}))

		tracer := obs.Tracer("github.com/aaronromeo/postmanpat/cli")
		meter := obs.Meter("github.com/aaronromeo/postmanpat/watchrunner")
		cycleCounter, _ := meter.Int64Counter("postmanpat.watch.cycles", metric.WithUnit("{cycle}"))
		cycleDuration, _ := meter.Float64Histogram("postmanpat.watch.cycle.duration", metric.WithUnit("s"))
		reconnectCounter, _ := meter.Int64Counter("postmanpat.watch.reconnects", metric.WithUnit("{reconnect}"))
		reloadCounter, _ := meter.Int64Counter("postmanpat.watch.config.reloads", metric.WithUnit("{reload}"))

		sessionID := newSessionID()
		connectCtx, connectSpan := tracer.Start(ctx, "watch.connect",
			trace.WithAttributes(attribute.String("watch.session.id", sessionID)))

		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		if strings.TrimSpace(testRuleName) != "" {
			connectSpan.End()
			if err := runWatchTest(cmd.Context(), client, cfg, logger, testRuleName, testMailbox, limit); err != nil {
				return err
			}
			return nil
		}

		selection, err := client.SelectMailbox(connectCtx, defaultMailbox)
		if err != nil {
			connectSpan.RecordError(err)
			connectSpan.SetStatus(codes.Error, err.Error())
			connectSpan.End()
			return err
		}
		connectSpan.End()
```

> Note: move the `client := watchrunner.New(...)` + `Connect()` block *below* the logger construction (it currently sits above). The `defer client.Close()` stays with the connect.

- [ ] **Step 4: Instrument the loop — reconnect, cycle, reload**

In the `for` loop:

1. Reconnect branch:

```go
		idleCmd, err := client.Idle()
		if err != nil {
			if watchrunner.IsBenignIdleError(err) {
				sessionID = newSessionID()
				rcCtx, rcSpan := tracer.Start(ctx, "watch.reconnect",
					trace.WithAttributes(attribute.String("watch.session.id", sessionID)))
				deps.Ctx = rcCtx
				if err := watchrunner.Reconnect(client, deps, state, defaultMailbox); err != nil {
					rcSpan.RecordError(err)
					rcSpan.SetStatus(codes.Error, err.Error())
					rcSpan.End()
					reconnectCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error")))
					return err
				}
				rcSpan.End()
				reconnectCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "success")))
				continue
			}
			return err
		}
```

2. `newCount := <-updateCh` branch (replace the existing body):

```go
			case newCount := <-updateCh:
				logger.Debug("idle update received", "messages", newCount, "last_messages", state.LastCount)
				_ = idleCmd.Close()
				if err := idleCmd.Wait(); err != nil {
					if !watchrunner.IsBenignIdleError(err) {
						return err
					}
				}
				if newCount > state.LastCount {
					cycleCtx, cycleSpan := tracer.Start(ctx, "watch.cycle",
						trace.WithAttributes(
							attribute.String("cycle.trigger", "new_mail"),
							attribute.String("watch.session.id", sessionID),
						))
					cycleStarted := time.Now()
					logger.Info("new mail detected", "messages", newCount)
					uids, err := client.SearchUIDsNewerThan(cycleCtx, state.LastUID)
					if err != nil {
						cycleSpan.RecordError(err)
						cycleSpan.SetStatus(codes.Error, err.Error())
						cycleSpan.End()
						cycleCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						cycleDuration.Record(ctx, time.Since(cycleStarted).Seconds(), metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						return err
					}
					deps.Ctx = cycleCtx
					if err := watchrunner.ProcessUIDs(client, deps, state, uids); err != nil {
						cycleSpan.RecordError(err)
						cycleSpan.SetStatus(codes.Error, err.Error())
						cycleSpan.End()
						cycleCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						cycleDuration.Record(ctx, time.Since(cycleStarted).Seconds(), metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "error")))
						return err
					}
					cycleSpan.End()
					cycleCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "success")))
					cycleDuration.Record(ctx, time.Since(cycleStarted).Seconds(), metric.WithAttributes(attribute.String("trigger", "new_mail"), attribute.String("outcome", "success")))
				}
				state.LastCount = newCount
				logger.Info("ready for next update")
```

3. `reloadTicker.C` branch — wrap in a `watch.config_reload` span. Open the branch with:

```go
			case <-reloadTicker.C:
				rlCtx, rlSpan := tracer.Start(ctx, "watch.config_reload",
					trace.WithAttributes(attribute.String("watch.session.id", sessionID)))
				logger.Debug("reload timer fired")
```

and change each `continue` in that branch to end the span and record the metric first:

```go
					logger.Error("watch config reload failed", "error", err)
					rlSpan.End()
					reloadCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "error")))
					continue
```

(same for all three validation failure paths), and at the end of the successful path:

```go
				cfg = updated
				deps.Rules = updated.Rules
				logger.Info("watch config reloaded")
				rlSpan.End()
				reloadCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "success")))
```

- [ ] **Step 5: Verify build and tests**

Run: `go build ./... && go test ./cli/ ./watchrunner/ ./ftest/`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cli/watch.go
git commit -m "feat(watch): emit connect, cycle, reconnect, and config-reload spans"
```

---

### Task 17: Watch trace integration test (in-memory IMAP + in-memory OTel)

**Files:**
- Create: `ftest/watchrunner_trace_integration_test.go`

- [ ] **Step 1: Write the test**

Create `ftest/watchrunner_trace_integration_test.go`:

```go
package ftest

import (
	"context"
	"io"
	"log/slog"
	"testing"

	appconfig "github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/watchrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWatchProcessUIDsEmitsMessageSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	client, ids, cleanup := setupWatchRunnerServer(t, []string{"Archive"})
	defer cleanup()

	if _, err := client.SelectMailbox(context.Background(), "INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}

	rule := appconfig.Rule{
		Name:   "MoveRule",
		Client: &appconfig.ClientMatchers{SenderRegex: []string{watchSenderHostPattern}},
		Actions: []appconfig.Action{{Type: appconfig.MOVE, Destination: "Archive"}},
	}
	deps := watchrunner.Deps{
		Ctx:   context.Background(),
		Rules: []appconfig.Rule{rule},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	state := &watchrunner.State{}
	require.NoError(t, watchrunner.ProcessUIDs(client, deps, state, []uint32{ids.NewsUID}))

	spans := rec.Ended()
	var msgSpan, actionSpan *sdktrace.ReadOnlySpan
	for i := range spans {
		switch spans[i].Name() {
		case "watch.message":
			msgSpan = &spans[i]
		case "watch.action":
			actionSpan = &spans[i]
		}
	}
	require.NotNil(t, msgSpan, "missing watch.message span")
	require.NotNil(t, actionSpan, "missing watch.action span")

	var sawMatchedEvent bool
	for _, ev := range msgSpan.Events() {
		if ev.Name != "watch.rule_evaluated" {
			continue
		}
		for _, kv := range ev.Attributes {
			if kv.Key == attribute.Key("matched") && kv.Value.AsBool() {
				sawMatchedEvent = true
			}
		}
	}
	assert.True(t, sawMatchedEvent, "expected a matched=true rule_evaluated event")
	assert.Equal(t, msgSpan.SpanContext().SpanID(), actionSpan.Parent().SpanID(), "action should be child of message span")
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./ftest/ -run TestWatchProcessUIDsEmitsMessageSpan -v`
Expected: PASS. (The in-memory server fetch also passes through the unwrapped runner here; the `watch.message`/`watch.action`/`watch.rule_evaluated` spans are what we assert.)

- [ ] **Step 3: Run the full ftest suite**

Run: `go test ./ftest/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ftest/watchrunner_trace_integration_test.go
git commit -m "test(watch): assert message span and rule evaluation events against in-memory IMAP"
```

---

## Phase 4 — Deployment to SigNoz

### Task 18: Pass OTLP env vars through `docker-compose.yml`

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add OTLP env vars to both postmanpat services**

In the `postmanpat` service `environment:` block add:

```yaml
      - OTEL_EXPORTER_OTLP_ENDPOINT=${OTEL_EXPORTER_OTLP_ENDPOINT:-http://signoz-otel-collector:4317}
      - OTEL_EXPORTER_OTLP_INSECURE=${OTEL_EXPORTER_OTLP_INSECURE:-true}
      - OTEL_EXPORTER_OTLP_HEADERS=${OTEL_EXPORTER_OTLP_HEADERS:-}
      - OTEL_SERVICE_NAME=${OTEL_SERVICE_NAME:-postmanpat}
```

In the `postmanpat-watch` service `environment:` block add the same four lines (service name may default to `postmanpat-watch` if you prefer; keep `postmanpat` for a single service in SigNoz).

- [ ] **Step 2: Validate YAML**

Run: `docker compose config --quiet`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "deploy: wire OTEL_* env vars into postmanpat and postmanpat-watch services"
```

---

### Task 19: Pass OTLP env vars through the cron entrypoint

**Files:**
- Modify: `docker/entrypoint.sh`

- [ ] **Step 1: Add OTLP vars to the crontab**

In `docker/entrypoint.sh`, inside the `cat >/etc/cron.d/postmanpat <<EOF` heredoc, add after the `POSTMANPAT_WEBHOOK_URL=...` line:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=${OTEL_EXPORTER_OTLP_ENDPOINT}
OTEL_EXPORTER_OTLP_INSECURE=${OTEL_EXPORTER_OTLP_INSECURE}
OTEL_EXPORTER_OTLP_HEADERS=${OTEL_EXPORTER_OTLP_HEADERS}
OTEL_SERVICE_NAME=${OTEL_SERVICE_NAME}
OTEL_SDK_DISABLED=${OTEL_SDK_DISABLED}
```

- [ ] **Step 2: Shellcheck the file**

Run: `sh -n docker/entrypoint.sh`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add docker/entrypoint.sh
git commit -m "deploy: pass OTLP env vars to the hourly cleanup cron job"
```

---

### Task 20: Document SigNoz setup

**Files:**
- Modify: `README.md`, `AGENTS.md`

- [ ] **Step 1: Add an Observability section to README.md**

Append a section after "Docker (Cleanup Cron)":

```markdown
## Observability (OpenTelemetry + SigNoz)

postmanpat sends OpenTelemetry traces and metrics to any OTLP/gRPC backend.
Self-hosted SigNoz on the same machine is the supported target.

Set these in `.env` before `docker compose up`:

```bash
# Self-hosted SigNoz: collector gRPC endpoint (http:// scheme = plaintext)
OTEL_EXPORTER_OTLP_ENDPOINT=http://signoz-otel-collector:4317
OTEL_EXPORTER_OTLP_INSECURE=true
# SigNoz Cloud instead: endpoint ingest.<region>.signoz.cloud:443 over TLS
# with OTEL_EXPORTER_OTLP_HEADERS=signoz-ingestion-key=<key>
```

Telemetry is enabled only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Traces:

- `watch` — a `watch.cycle` span per new-mail batch, a `watch.message` span per
  email with one `watch.rule_evaluated` event per rule (`matched` attr), and a
  `watch.action` span per applied action.
- `cleanup` — one `cleanup.invocation` root span per run, with
  `cleanup.rule` and `cleanup.action` children and one `action.message_identified`
  event per matched email.
```

- [ ] **Step 2: Update AGENTS.md OTel Status**

Replace the "OTel Status" section with:

```markdown
## OTel Status
- Implemented: `obs/` package (Init, env config, resource, `WrapCleanupRunner`, `WrapWatchRunner`), trace instrumentation for `watch` (cycle/message/rule_evaluated/action) and `cleanup` (invocation/rule/action + per-message events), IMAP RED metrics, OTLP gRPC export (plaintext via `OTEL_EXPORTER_OTLP_INSECURE` or `http://` scheme), docker-compose + cron OTLP env wiring.
- Deferred: slog→OTel logs bridge (see `docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md` §3.3), OTLP/HTTP exporter.
```

- [ ] **Step 3: Commit**

```bash
git add README.md AGENTS.md
git commit -m "docs: document SigNoz OTLP configuration and observability status"
```

---

### Task 21: Manual verification against SigNoz (ops checklist, not automated)

**Files:** none

- [ ] **Step 1: Build and deploy**

On the DigitalOcean machine:

```bash
git submodule update --init --recursive
docker compose build
docker compose up -d
```

- [ ] **Step 2: Verify cleanup traces**

```bash
docker compose run --rm postmanpat postmanpat cleanup --config /config/config_cleanup.yaml
```

In SigNoz, query `service.name = postmanpat`, operation `cleanup.invocation`. Confirm: one root span per run; `cleanup.rule` children with `rule.name` and `rule.matched_count`; `cleanup.action` children with `action.type`; `action.message_identified` events visible on matched actions; `imap.*` children under each rule/action.

- [ ] **Step 3: Verify watch traces**

Send a test email to the watched inbox (or `docker compose logs -f postmanpat-watch` to confirm an update arrives). In SigNoz, filter by operation `watch.cycle` / `watch.message`. Confirm each email has a `watch.message` span with one `watch.rule_evaluated` event per configured rule and the correct `matched` values, plus `watch.action` spans for matched rules. Confirm the `watch.session.id` attribute groups spans from the same IMAP session.

- [ ] **Step 4: Verify metrics**

In SigNoz Metrics, confirm `postmanpat.imap.operations`, `postmanpat.cleanup.*`, and `postmanpat.watch.*` series appear.

- [ ] **Step 5: Verify disable behavior**

Set `OTEL_EXPORTER_OTLP_ENDPOINT=` in `.env`, restart, and confirm the app still runs with zero OTLP traffic (no-op providers).

---

## Phase 5 — Final verification

### Task 22: Full verification pass

**Files:** none

- [ ] **Step 1: Format check**

Run: `gofmt -l .`
Expected: no files listed.

- [ ] **Step 2: Vet**

Run: `go vet ./...`
Expected: exits 0.

- [ ] **Step 3: Full test suite**

Run: `go test ./...`
Expected: all 71+ tests pass (existing plus the new obs/cli/ftest/watchrunner tests).

- [ ] **Step 4: Build**

Run: `go build -o bin ./...`
Expected: `bin/postmanpat` produced.

- [ ] **Step 5: Confirm working tree clean of stray artifacts**

Run: `git status`
Expected: no unexpected modified/untracked files (the gitignored `bin/postmanpat` may appear; leave it).

---

## Self-review notes

- **Spec coverage:** Every requirement in the user's request maps to a task: watch trace with rule comparison and match outcome → Tasks 15–17; cleanup trace with per-email rule matches and actions → Tasks 12–13; testing → Tasks 13, 15, 17, 22; SigNoz deployment → Tasks 18–21. The existing spec's trace shapes (§3.1, §3.4) are implemented; §4.3 deviation recorded in Task 2.
- **Consistency:** Span names, attribute keys, and metric names match the "Canonical contract" table exactly; `ProcessUIDs`/`Reconnect` signatures are used identically across Tasks 14–17; `FetchSenderDataByMailbox` appears on the interface (Task 11) and the decorator (Task 7) with the same name.
- **Dependency order:** Task 7's decorator references the interface method added in Task 11; the plan calls this out and both land before any full-suite run in Task 11.
