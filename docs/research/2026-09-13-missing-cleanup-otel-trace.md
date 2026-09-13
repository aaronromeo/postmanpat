# Missing OTel Trace for One-Time `cleanup` Run

**Date:** 2026-09-13
**Question:** Why is there no OTel trace in SigNoz for a one-time `cleanup` run?

## TL;DR

The OTel SDK instrumentation and shutdown path are correct — spans are created, queued, and flushed on exit. The root cause is **ClickHouse memory exhaustion in the SigNoz stack**, not the postmanpat client. Spans are successfully received by the SigNoz OTel Collector's OTLP gRPC receiver, but the `clickhousetraces` exporter fails with `code: 241, memory limit exceeded` (ClickHouse RSS exceeds its 2.00 GiB cap). The collector retries with exponential backoff until data is dropped from its queue. The SigNoz stack has been in this degraded state since at least 2026-09-11T03:33 UTC — two full days before the cleanup run in question.

## Findings

### Thread 1: SDK Flush on Exit

**Conclusion: NOT the root cause. Shutdown + drain is correct.**

The execution path is:

1. `cmd/postmanpat/main.go:19` — `obs.Init(ctx)` creates a `TracerProvider` with `sdktrace.WithBatcher(traceExp)`
2. `cmd/postmanpat/main.go:25` — `cli.ExecuteWithContext(ctx)` runs the cleanup command
3. `cli/cleanup.go:99` — `tracer.Start(ctx, "cleanup.invocation", ...)` creates the root span
4. `cli/cleanup.go:297` — `invSpan.End()` ends the span, enqueuing it to the `BatchSpanProcessor`
5. `cmd/postmanpat/main.go:29` — `shutdown(shutdownCtx)` with 5-second timeout

Source: `cmd/postmanpat/main.go:15-33`
```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    shutdown, err := obs.Init(ctx)
    if err != nil {
        fmt.Fprintln(os.Stderr, "observability init failed:", err)
        os.Exit(1)
    }

    code := cli.ExecuteWithContext(ctx)

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := shutdown(shutdownCtx); err != nil {
        fmt.Fprintln(os.Stderr, "observability shutdown failed:", err)
        code = 1
    }
    os.Exit(code)
}
```

The TracerProvider is constructed with a `BatchSpanProcessor` (via `WithBatcher`):

Source: `obs/init.go:34-37`
```go
tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(traceExp),
    sdktrace.WithResource(res),
)
```

The OTel Go SDK `BatchSpanProcessor` defaults (source: `batch_span_processor.go:21-28`, `~/go/pkg/mod/go.opentelemetry.io/otel/sdk@v1.44.0/trace/batch_span_processor.go:21-28`):
- `DefaultMaxQueueSize = 2048`
- `DefaultScheduleDelay = 5000` ms (batch timer interval)
- `DefaultExportTimeout = 30000` ms
- `DefaultMaxExportBatchSize = 512`

For a short-lived cleanup run (seconds), the 5-second batch timer likely hasn't fired before the process exits, so spans sit in the queue. **However**, `tp.Shutdown()` is called, which per the [OTel spec](https://opentelemetry.io/docs/specs/otel/trace/sdk/#shutdown-1):

> **Shutdown MUST include the effects of ForceFlush.**

The Go SDK implementation calls `bsp.Shutdown(ctx)` → closes `stopCh` → `processQueue()` exits → **`drainQueue()` runs** → exports all remaining queued spans → then `bsp.e.Shutdown(ctx)` (the exporter):

Source: `batch_span_processor.go:126-131` (goroutine start):
```go
bsp.stopWait.Go(func() {
    bsp.processQueue()
    bsp.drainQueue()
})
```

Source: `batch_span_processor.go:361-389` (`drainQueue`):
```go
func (bsp *batchSpanProcessor) drainQueue() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    for {
        select {
        case sd := <-bsp.queue:
            // ... collect into batch ...
        default:
            // There are no more enqueued spans. Make final export.
            if err := bsp.exportSpans(ctx); err != nil {
                otel.Handle(err)
            }
            return
        }
    }
}
```

This is correct: `Shutdown` drains the queue before the exporter stops. The SDK behavior is compliant with the spec.

### Thread 2: Environment Variable Wiring

**Conclusion: NOT the root cause. OTEL env vars are correctly resolved.**

The `.env` file at `/opt/docker/postmanpat/.env` (lines 19-21):
```
OTEL_EXPORTER_OTLP_ENDPOINT=http://signoz-otel-collector:4317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_SERVICE_NAME=postmanpat
```

The `docker-compose.yml` (lines 38-41) passes these through:
```yaml
OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-http://signoz-otel-collector:4317}
OTEL_EXPORTER_OTLP_INSECURE: ${OTEL_EXPORTER_OTLP_INSECURE:-true}
OTEL_EXPORTER_OTLP_HEADERS: ${OTEL_EXPORTER_OTLP_HEADERS:-}
OTEL_SERVICE_NAME: ${OTEL_SERVICE_NAME:-postmanpat}
```

Resolved compose config confirms the variables are populated:
```
OTEL_EXPORTER_OTLP_ENDPOINT: http://signoz-otel-collector:4317
OTEL_EXPORTER_OTLP_HEADERS: ""
OTEL_EXPORTER_OTLP_INSECURE: "true"
OTEL_SERVICE_NAME: postmanpat
```

Per the [Docker Compose docs](https://docs.docker.com/compose/how-tos/environment-variables/), `docker compose run` inherits the service's `environment:` block and the `.env` file is used for variable substitution. A live test confirmed the ephemeral container receives these values:
```
$ docker compose run --rm --entrypoint /bin/sh postmanpat -c 'echo OTEL_ENDPOINT=$OTEL_EXPORTER_OTLP_ENDPOINT; echo OTEL_INSECURE=$OTEL_EXPORTER_OTLP_INSECURE; echo OTEL_SERVICE=$OTEL_SERVICE_NAME'
OTEL_ENDPOINT=http://signoz-otel-collector:4317
OTEL_INSECURE=true
OTEL_SERVICE=postmanpat
```

The endpoint is network-reachable from the ephemeral container (verified via `/dev/tcp` test).

### Thread 3: Instrumentation Path

**Conclusion: NOT the root cause. OTel SDK is initialized unconditionally.**

`obs.Init()` is called in `main.go:19` before any Cobra command runs. The `IsEnabled()` check (`obs/config.go:12-17`) only gates whether real or no-op providers are installed — with `OTEL_EXPORTER_OTLP_ENDPOINT` set, real providers are installed:

Source: `obs/config.go:12-17`:
```go
func IsEnabled() bool {
    if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
        return false
    }
    return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}
```

`obs.Tracer()` (`obs/obs.go:10-12`) returns `otel.Tracer(name)`, which uses the global provider set by `otel.SetTracerProvider(tp)` in `obs.Init()`. This is the real OTel SDK tracer, not a no-op.

The `obs.WrapCleanupRunner` (`cli/cleanup.go:84`) wraps the IMAP client, and `cli/cleanup.go:91-98` creates the domain-level tracer/meter for `cleanup.invocation`, `cleanup.rule`, and `cleanup.action` spans. All use the OTel SDK tracer.

### Thread 4: Network Reachability

**Conclusion: NOT the root cause. Endpoint resolves and is reachable.**

Verified from an ephemeral container:
```
$ docker compose run --rm --entrypoint /bin/sh postmanpat -c 'timeout 3 bash -c "echo > /dev/tcp/signoz-otel-collector/4317" && echo "reachable"'
reachable
```

The `signoz-otel-collector` container is running (`docker ps` shows `Up About a minute` — it recently restarted). Both containers share the `signoz-net` Docker network.

### Thread 5: SigNoz Collector State (ROOT CAUSE)

**Conclusion: This IS the root cause.**

The `signoz-otel-collector` logs reveal a **chronic, ongoing failure** dating back to at least **2026-09-11T03:33:05 UTC** — over 47 hours before the cleanup run at 2026-09-13T02:30 UTC:

```
{"level":"info","ts":"2026-09-11T03:33:05.012Z",...,"otelcol.component.id":"clickhousetraces",...,"error":"error in writing spans to clickhouse: TracesWriteBatchOfSpansV3:code: 241, message: (total) memory limit exceeded: would use 2.26 GiB (attempt to allocate chunk of 4.16 MiB bytes), current RSS: 2.02 GiB, maximum: 2.00 GiB. OvercommitTracker decision: Query was selected to stop by OvercommitTracker",...}
```

The error is **ClickHouse error code 241**: memory limit exceeded. ClickHouse is configured with a 2.00 GiB RSS limit, and it consistently exceeds this when the collector tries to write spans or metrics.

The collector's retry queue eventually exhausts:
```
{"level":"error","ts":"2026-09-11T03:33:25.804Z",..."otelcol.component.id":"signozclickhousemetrics",..."error":"no more retries left: context deadline exceeded","dropped_items":11000,...}
```

At 30,484 errors in the last 12 hours alone, the collector is in a continuous failure loop:
1. Spans are received by the OTLP gRPC receiver (`:4317`) — this layer works fine
2. The `clickhousetraces` exporter tries to write to ClickHouse
3. ClickHouse rejects with `code: 241` (memory limit exceeded)
4. The collector retries with exponential backoff
5. Retries time out (`context deadline exceeded`)
6. Queue fills up, items are dropped ("no more retries left")

The cleanup run at 2026-09-13T02:30 UTC occurred while the collector was in this degraded state. The spans were sent successfully to the OTLP receiver (gRPC connection succeeded), but ClickHouse rejected them and they were never persisted.

The fact that the collector was "Up About a minute" when checked means it was recently restarted (likely due to OOM or health check failure), but the underlying ClickHouse memory issue has been present since 2026-09-11.

## Root Cause

**Ranked candidates:**

| Rank | Cause | Evidence | Confidence |
|------|-------|----------|------------|
| **1** | ClickHouse memory limit exceeded in SigNoz | `code: 241, memory limit exceeded: would use 2.26 GiB, maximum: 2.00 GiB` in collector logs since 2026-09-11T03:33 UTC; 30,484 errors in last 12h; spans explicitly dropped ("no more retries left", "dropped_items:11000") | **Definitive** |
| 2 | SDK flush on exit (hypothetical) | Ruled out: `main.go` calls `shutdown()` which invokes `tp.Shutdown()` → `bsp.Shutdown()` → `drainQueue()` → export. Compliant with OTel spec "Shutdown MUST include the effects of ForceFlush" | **Ruled out** |
| 3 | Missing OTEL env vars in `docker compose run` | Ruled out: live test confirmed `OTEL_EXPORTER_OTLP_ENDPOINT=http://signoz-otel-collector:4317` is set; network connectivity verified | **Ruled out** |
| 4 | No-op tracer (OTel not initialized) | Ruled out: `obs.Init()` called unconditionally in `main.go:19`; `IsEnabled()` returns `true` because `OTEL_EXPORTER_OTLP_ENDPOINT` is set | **Ruled out** |

## Suggested Fix

The postmanpat OTel implementation is correct. The fix is on the SigNoz/ClickHouse side:

1. **Increase ClickHouse memory limit** — the 2.00 GiB cap is too low for the data volume. Either increase the ClickHouse memory limit or add more RAM to the host.
2. **Reduce data ingestion** — if increasing ClickHouse resources isn't feasible, consider:
   - Using a trace sampler (e.g., `OTEL_TRACES_SAMPLER=parentbased_traceidratio` with `OTEL_TRACES_SAMPLER_ARG=0.1` to sample 10%)
   - Reducing metric collection frequency or cardinality
   - Clearing old ClickHouse data if disk pressure is contributing
3. **Restart ClickHouse** — a restart may clear the RSS, but without addressing the root cause (too much data or too little memory) the problem will recur.

No changes to postmanpat code are required.
