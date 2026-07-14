# OpenTelemetry Foundation + Cleanup & Watch Instrumentation — Design

**Status:** Draft (in progress)
**Date:** 2026-06-13
**Scope:** Foundation (the `obs` package, SDK wiring, slog bridge) plus full instrumentation of both the `cleanup` and `watch` commands. Watch is included because it is the runtime mode of highest debugging interest.

> Sections are appended as they are approved during brainstorming. Unapproved sections are marked `(pending)`.

---

## 1. Architecture overview

A new `obs/` package owns all OpenTelemetry concerns: provider construction, env-driven configuration, the slog→OTel log bridge, graceful shutdown, and instrumentation decorators for narrow interfaces. `cmd/postmanpat/main.go` calls `obs.Init(ctx)` before Cobra runs, gets back a `shutdown` function, and defers it. Cobra commands (`cleanup`, `watch`) acquire tracers/meters via the OTel globals that `Init` registers (`otel.Tracer("...")`, `otel.Meter("...")`).

For the noisy boundaries that warrant it — the IMAP `serverrunner.ServerRunner` interface (cleanup) and the IMAP `watchrunner.WatchRunner` interface (watch), whose methods are invoked inside hot loops — `obs` exposes `WrapCleanupRunner(...)` and `WrapWatchRunner(...)`. The wrapper names follow the consumer (cleanup, watch), not the underlying interface type. The cobra commands compose the instrumented decorators around the real clients at construction time. Runners and call sites are unchanged — they still depend on the same interfaces. This is the Approach-A facade for top-level/semantic spans, plus the Approach-C decorator pattern at the two seams where per-call latency and error metrics genuinely earn their cost.

The announcer (webhook) is intentionally **not** wrapped. It is invoked at most a few times per cleanup run, so RED metrics would be noise. Its calls already live inside the per-rule span, so they appear in traces for free, and any failures emitted via the existing `logger.Error("reporting failed", ...)` call site will carry the trace context via the slog → OTel bridge. A wrapper can be added later if webhook reliability ever becomes an operational concern; the interface seam is already there.

When `OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_SDK_DISABLED=true`) is not set, `obs.Init` installs no-op providers — zero network traffic, zero allocations beyond the no-op span/metric/log structs, and the existing stdout slog handler keeps working as it does today.

S3 archival is not implemented in this branch; this spec leaves a documented seam (`obs.WrapArchiveClient`) for when it lands, but does not introduce S3 code.

### Key answered decisions

| Decision | Choice |
|---|---|
| Telemetry signals | Full three: traces, metrics, logs |
| Backend | OTLP-only, backend-agnostic |
| Topology | Direct OTLP export from the app (no collector required) |
| Default behavior | Disabled (no-op providers) when OTLP endpoint env var unset |
| Cleanup trace shape | One root span per invocation; child spans per rule, mailbox, action |
| Watch trace shape | Per-cycle root spans (new_mail, reconnect, config_reload); long-lived IDLE wait is not a span |
| Metrics scope | RED (rate/errors/duration) per dependency + domain counters |
| Logs strategy | Bridge slog → OTel logs; keep stdout text output as well |
| Wiring | New `obs/` package, initialized in `cmd/postmanpat/main.go` |
| Configuration | Standard `OTEL_*` env vars only (no YAML knobs) |
| Pattern | Hybrid: facade + decorators on narrow interface seams |
| Wrapper application | Wrappers live in `obs`, applied in the cobra command at construction time |

---

## 2. The `obs` package

### Public API

```go
package obs

// Init configures OTel SDK providers from environment variables and
// registers them as OTel globals. Returns a shutdown function that
// flushes and stops providers; safe to call multiple times.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset (or OTEL_SDK_DISABLED=true),
// Init installs no-op providers and returns a no-op shutdown. No network.
func Init(ctx context.Context) (shutdown func(context.Context) error, err error)

// NewSlogHandler wraps an existing slog.Handler so records are:
//   1. emitted to the wrapped handler (stdout text), AND
//   2. bridged to the OTel logs provider when enabled.
// Trace and span IDs from ctx are attached as record attributes.
func NewSlogHandler(base slog.Handler) slog.Handler

// WrapCleanupRunner returns an instrumented serverrunner.ServerRunner
// for use by the cleanup command. Each interface method becomes a child
// span with attributes (mailbox, uid count, destination, expunge) and
// records duration as a histogram and errors as counters.
func WrapCleanupRunner(inner serverrunner.ServerRunner) serverrunner.ServerRunner

// WrapWatchRunner returns an instrumented watchrunner.WatchRunner for
// use by the watch command. Same pattern as WrapCleanupRunner; emits to
// the shared postmanpat.imap.* metric set (operation label differentiates).
func WrapWatchRunner(inner watchrunner.WatchRunner) watchrunner.WatchRunner

// Tracer returns a tracer scoped to the given instrumentation name.
// Thin wrapper over otel.Tracer for readability.
func Tracer(name string) trace.Tracer

// Meter returns a meter scoped to the given instrumentation name.
func Meter(name string) metric.Meter
```

### Internal layout

- `obs/init.go` — `Init` and `shutdown` orchestration.
- `obs/config.go` — env-var inspection (read-only of `OTEL_*`); decides which signals are enabled and whether the SDK is disabled.
- `obs/resource.go` — builds the OTel `Resource` with `service.name=postmanpat`, `service.version` (from build info or ldflags), `service.instance.id` (random UUID per process), and `process.command` (the subcommand: `cleanup`, `watch`, etc., set by the cobra command before `Init`).
- `obs/slog_bridge.go` — `NewSlogHandler`, fan-out to base handler + OTel logs bridge. Trace/span IDs injected from `ctx` on each `Handle`.
- `obs/cleanuprunner.go` — decorator implementing `serverrunner.ServerRunner` (used by cleanup) with tracer + meter.
- `obs/watchrunner.go` — decorator implementing `watchrunner.WatchRunner` (used by watch) with tracer + meter. Shares a small internal helper with `obs/cleanuprunner.go` for the span+metric+error pattern.
- `obs/internal/` — only if needed for shared attribute keys / helpers.

Each file has a colocated `_test.go` exercising the wrapper or helper in isolation against in-memory exporters (`sdktrace.NewTracerProvider` with `tracetest.InMemoryExporter`, plus the metrics and logs equivalents from `go.opentelemetry.io/otel/sdk/...`).

### Dependencies added to `go.mod`

- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/sdk`
- `go.opentelemetry.io/otel/sdk/metric`
- `go.opentelemetry.io/otel/sdk/log`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`
- `go.opentelemetry.io/otel/log` (logs API, stable)
- `go.opentelemetry.io/contrib/bridges/otelslog` (official slog → OTel logs bridge)

gRPC OTLP only for v1 (no HTTP variant). `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` is implied. HTTP support is an additive change later if needed.

---

## 3. Trace, metric, and log semantics for `cleanup`

### 3.1 Traces

One root span per invocation of `postmanpat cleanup`, started in `cli/cleanup.go` immediately after config validation and before the IMAP connection. The shape:

```
cleanup.invocation                                          [root]
├── cleanup.connect                                         (IMAP Connect)
├── cleanup.rule  rule.name=<rule>                          (one per rule)
│   ├── imap.search_by_server_matchers                      (from ServerRunner decorator)
│   ├── announcer.do                                        (in-line, no decorator)
│   └── cleanup.action  action.type=<delete|move>           (one per action)
│       ├── (event) action.fetch_identity  uid_count=<n>
│       ├── imap.fetch_sender_data                          (from decorator)
│       ├── (event) action.message_identified               (one per message)
│       │     attrs: imap.uid, email.message_id,
│       │            email.from, email.subject,
│       │            email.internal_date
│       ├── (event) action.applied  uid_count=<n>, dry_run=<bool>
│       └── imap.move_by_mailbox / imap.delete_by_mailbox   (from decorator)
└── cleanup.disconnect                                      (IMAP Close, deferred)
```

**Span attributes:**

- `cleanup.invocation`: `postmanpat.command=cleanup`, `postmanpat.dry_run=<bool>`, `postmanpat.config_path=<path>`, `postmanpat.rules.count=<n>`. On finish, also `postmanpat.rules.matched=<n>`, `postmanpat.messages.matched=<total>`.
- `cleanup.rule`: `rule.name`, `rule.mailbox`, `rule.actions=[delete,move,...]`. On finish: `rule.matched_count`.
- `cleanup.action`: `action.type` (`delete` or `move`), `action.destination` (for move), `action.expunge` (for delete), `action.dry_run`, `action.uid_count`.
- IMAP spans (from the decorator): `imap.operation` (`search`/`move`/`delete`/`fetch`), `imap.mailbox`, `imap.uid_count`, `imap.destination` (for move), `imap.expunge` (for delete).

**Per-message audit trail (span events on `cleanup.action`):**

For every action that has matched UIDs, the cleanup flow becomes:

1. Start `cleanup.action` span.
2. Call `client.FetchSenderDataByMailbox(ctx, matched)` to get per-mailbox `MailData` (UID + headers). This method already exists at `imap/internal/selectors/manager.go:71`.
3. For each returned `MailData` entry, emit `span.AddEvent("action.message_identified", trace.WithAttributes(...))` with attributes:
   - `imap.uid` (uint32)
   - `email.message_id` (the RFC 5322 `Message-ID` header)
   - `email.from`
   - `email.subject`
   - `email.internal_date` (IMAP `INTERNALDATE`, RFC 3339 string)
4. Issue the bulk move/delete (existing call).
5. End span, record outcome.

If the rule matched zero messages, the action is a no-op and no fetch/events are emitted.

**Error handling:** every span uses `span.RecordError(err)` + `span.SetStatus(codes.Error, msg)` on failure. Top-level errors propagate to the root span, so a failed run is one click to find.

**Known scaling boundary (deliberate):** Per-message events are unsampled — OTel sampling is per-span, not per-event. A rule that matches 5,000 messages produces one span with 5,000 events. Most backends handle hundreds comfortably; some push back in the thousands. If/when this becomes a real problem, the escape hatch is a cap-and-summarize variant (fetch a capped slice, emit a `messages_truncated=true` summary event with totals). Not implemented in v1.

**Measured cost:** one additional IMAP round-trip per action (envelope-only fetch, no body). For typical Gmail mailboxes this is cheap. The fetch goes through the instrumented `ServerRunner` decorator, so its latency is captured in `postmanpat.imap.duration{operation="fetch"}`.

### 3.2 Metrics

All metrics use the meter named `github.com/aaronromeo/postmanpat/cleanuprunner` (for cleanup-emitted instruments) and `github.com/aaronromeo/postmanpat/obs/cleanuprunner` (for IMAP decorator instruments).

**Cleanup-domain instruments (emitted from `cli/cleanup.go`):**

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `postmanpat.cleanup.invocations` | Counter | `{run}` | `outcome` (`success`/`error`), `dry_run` |
| `postmanpat.cleanup.duration` | Histogram | `s` | `outcome`, `dry_run` |
| `postmanpat.cleanup.rule.matches` | Counter | `{message}` | `rule.name`, `mailbox` |
| `postmanpat.cleanup.action.messages` | Counter | `{message}` | `action.type` (`delete`/`move`), `rule.name`, `destination` (empty for delete), `dry_run` |
| `postmanpat.cleanup.action.errors` | Counter | `{error}` | `action.type`, `rule.name` |

**IMAP-decorator instruments (RED, emitted from `obs/cleanuprunner.go` and `obs/watchrunner.go`, shared instrument set):**

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `postmanpat.imap.operations` | Counter | `{op}` | `operation` (`connect`/`close`/`search`/`fetch`/`move`/`delete`), `outcome` |
| `postmanpat.imap.duration` | Histogram | `s` | `operation`, `outcome` |
| `postmanpat.imap.errors` | Counter | `{error}` | `operation` |

**Cardinality discipline:**

- `rule.name` — bounded by the YAML config (typically < 50). Acceptable.
- `mailbox` — bounded by IMAP folders touched per run. Acceptable.
- `destination` — bounded by the union of move targets in the config. Acceptable.
- Per-message identifiers (`message_id`, `sender`, `subject`, `uid`) are **never** used as metric attributes. They live on span events and in logs.

Histograms use OTel SDK default latency boundaries; no custom views in v1.

### 3.3 Logs

`cli/cleanup.go` constructs its slog logger via `obs.NewSlogHandler(stdoutTextHandler)` instead of the current `slog.NewTextHandler(out, ...)` directly. The wrapper:

1. Forwards every record to the underlying text handler — stdout output stays byte-identical to today's behavior.
2. When the OTel logs provider is enabled, bridges the record via `otelslog` so it becomes an OTel `LogRecord` with `severity`, `body`, all slog attributes, and the active `trace_id` / `span_id` from the context if one is in scope.

To get trace correlation, the existing call sites must use the context-aware slog methods (`logger.InfoContext(ctx, ...)`, `logger.ErrorContext(ctx, ...)`) rather than the bare `logger.Info(...)`. This is a mechanical change in `cli/cleanup.go` (about 8 call sites). The bare-method calls still compile and work — they just won't carry trace IDs.

No new log lines are introduced by this design. Per-message identity is recorded as span events (section 3.1), not as log records. Existing log statements get richer correlation; that is the entirety of the logs work in this spec for `cleanup`.

### 3.4 Watch-mode instrumentation

#### Decorator extension

Add `WrapWatchRunner(inner watchrunner.WatchRunner) watchrunner.WatchRunner` to the `obs` package. Same pattern as `WrapCleanupRunner` — wraps every interface method with a span + RED metrics.

| Method | Span name | Metric op label |
|---|---|---|
| `Connect` | `imap.connect` | `connect` |
| `Close` | `imap.close` | `close` |
| `Idle` | `imap.idle.start` | `idle_start` (only the issuance of IDLE; the wait happens outside the decorator) |
| `SelectMailbox` | `imap.select` | `select` |
| `FetchSenderData` | `imap.fetch_sender_data` | `fetch` |
| `SearchUIDsNewerThan` | `imap.search_newer_than` | `search` |
| `MoveUIDs` | `imap.move` | `move` |
| `DeleteUIDs` | `imap.delete` | `delete` |

All emit to the shared `postmanpat.imap.*` instrument set (`operations` counter, `duration` histogram, `errors` counter). The `operation` label differentiates. Both decorators may share an internal helper for the span+metric+error pattern.

#### Trace shape — per-cycle root spans

Long-lived spans defeat OTel's batch flushing, so the watch loop does **not** wrap the IDLE wait. Instead, each event that breaks out of the wait produces its own short-lived root span:

```
watch.cycle  cycle.trigger=<new_mail|reconnect|reload>          [root]
├── imap.search_newer_than    (uid_count attribute)
├── imap.fetch_sender_data    (uid_count attribute)             (from ProcessUIDs)
└── watch.message                                               (one per message)
    │     attrs: imap.uid, email.message_id, email.from,
    │            email.subject, email.internal_date
    ├── (event) watch.rule_evaluated rule.name=<n> matched=<bool>   (one per rule)
    └── watch.action  action.type=<delete|move>                 (only when a rule matched)
        └── imap.move / imap.delete
```

The three non-cycle operations get their own short-lived root spans:

- `watch.connect` — wraps initial `client.Connect()` + first `SelectMailbox` at startup.
- `watch.reconnect` — wraps `client.Reconnect(...)` after a benign idle error.
- `watch.config_reload` — wraps the reload-ticker branch (close idle, reload YAML, validate, swap rules).

#### Session attribution

To group cycles by IMAP session without using a long-lived span, the watcher generates a `watch.session.id` (random UUID) on each successful connect/reconnect. Every root span (`watch.cycle`, `watch.connect`, `watch.reconnect`, `watch.config_reload`) sets that attribute. Span links are not used in v1 — a string attribute is enough to group/filter by session in any backend. If the simple attribute proves insufficient, span links can be added later as an additive change.

#### Watch-specific metrics

In addition to the shared `postmanpat.imap.*` set:

| Instrument | Type | Unit | Attributes |
|---|---|---|---|
| `postmanpat.watch.cycles` | Counter | `{cycle}` | `trigger` (`new_mail`/`reconnect`/`reload`), `outcome` |
| `postmanpat.watch.cycle.duration` | Histogram | `s` | `trigger`, `outcome` |
| `postmanpat.watch.messages.processed` | Counter | `{message}` | (no per-rule label here) |
| `postmanpat.watch.rule.matches` | Counter | `{message}` | `rule.name` |
| `postmanpat.watch.action.messages` | Counter | `{message}` | `action.type`, `rule.name`, `destination` |
| `postmanpat.watch.idle.disconnects` | Counter | `{disconnect}` | `kind` (`benign`/`error`) |
| `postmanpat.watch.reconnects` | Counter | `{reconnect}` | `outcome` |
| `postmanpat.watch.config.reloads` | Counter | `{reload}` | `outcome` |
| `postmanpat.watch.session.duration` | Histogram | `s` | `outcome` (recorded on session end / reconnect) |

Cardinality discipline is identical to cleanup: `rule.name` / `destination` come from the YAML config (bounded); nothing per-message ever lands in metric attributes.

#### Per-message audit on `watch.message`

Symmetric with `cleanup.action`'s per-message events, but with a different shape: watch processes one message at a time inside the loop, so each message gets its own `watch.message` span (not events on a shared parent). The identity bundle (`imap.uid`, `email.message_id`, `email.from`, `email.subject`, `email.internal_date`) is set as span **attributes** directly. Per-rule evaluation outcomes become events on the same span: `watch.rule_evaluated { rule.name, matched }`.

#### Logs

`cli/watch.go` constructs its top-level logger via `obs.NewSlogHandler(...)`, passes it through `watchrunner.Deps.Log` (already exists). Call sites in both `cli/watch.go` and `watchrunner/runner.go` move to `*Context` slog methods. `watchrunner.ProcessUIDs` and `applyActions` use `deps.Ctx` for log correlation (the context is already available in `Deps`).

No new log lines are introduced. Existing log statements gain trace correlation.

#### Scope clarifications

- The `--test` mode shares the IMAP client construction in `cli/watch.go`, so it passively benefits from the decorator and slog bridge. No dedicated test-mode spans in v1.
- The IDLE wait itself (the `select` block on `updateCh`/`ctx.Done()`/`reloadTicker.C`) is **not** wrapped in a span — it lives for minutes to hours. Its triggers produce the per-cycle root spans.
- The `announcer.Do(...)` call inside `Deps.Announce` is not wrapped (same reasoning as cleanup); it appears in the `watch.message` or `watch.cycle` timeline by virtue of being called inside that span.

---

## 4. Configuration surface (`OTEL_*` env vars)

### 4.1 The "is OTel enabled?" decision

`obs.Init` enables OTel iff **all** of these are true:

1. `OTEL_SDK_DISABLED` is not set to `true` (case-insensitive).
2. `OTEL_EXPORTER_OTLP_ENDPOINT` is set and non-empty (after trim).

Either condition failing → no-op providers, no network, no allocations beyond the no-op structs. The slog handler still fans out, but its OTel side becomes a no-op.

`OTEL_SDK_DISABLED` is the spec-defined kill switch from the OTel SDK; honoring it is the contract for being a well-behaved OTel app. `OTEL_EXPORTER_OTLP_ENDPOINT` being our presence sentinel matches what every OTel-Go SDK example does and avoids inventing a custom `POSTMANPAT_OTEL_ENABLED` knob.

### 4.2 Env vars honored

PostmanPat itself does not read these — the OTel SDK does. The spec commits to passing them through to the SDK constructors. Documented here because they are what an operator will set:

| Variable | Purpose | Notes |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | gRPC endpoint (e.g., `https://otlp.example.com:4317`) | Required for OTel to enable |
| `OTEL_EXPORTER_OTLP_HEADERS` | Auth headers (e.g., `api-key=...`) | Comma-separated `k=v` pairs per OTel spec |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | Wire protocol | Only `grpc` supported in v1; defaults to `grpc` if unset |
| `OTEL_EXPORTER_OTLP_COMPRESSION` | `gzip` or `none` | Default per SDK |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | Per-export timeout (ms) | Default per SDK |
| `OTEL_SDK_DISABLED` | Kill switch | `true` = no-op providers even if endpoint set |
| `OTEL_SERVICE_NAME` | Service name override | Defaults to `postmanpat` (set in §5) |
| `OTEL_SERVICE_VERSION` | Version override | Defaults from build info (§5) |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes | Comma-separated `k=v`; merged with defaults |
| `OTEL_TRACES_SAMPLER` | Sampler choice | Default: `parentbased_always_on` |
| `OTEL_TRACES_SAMPLER_ARG` | Sampler argument | E.g., ratio for `parentbased_traceidratio` |
| `OTEL_LOG_LEVEL` | SDK internal log level | For debugging the SDK itself |
| `OTEL_PROPAGATORS` | Context propagators | Default: `tracecontext,baggage` |

Per-signal endpoint/header overrides (`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, etc.) are honored because the SDK reads them automatically — no extra code.

### 4.3 Variables explicitly not honored

- **`OTEL_EXPORTER_OTLP_INSECURE`** — not honored in v1. PostmanPat's OTLP endpoints are expected to be TLS-protected; the gRPC exporter defaults to TLS when the endpoint scheme is `https://`. Add as an additive change if a plaintext use case appears.
- **`OTEL_METRIC_EXPORT_INTERVAL` / `OTEL_METRIC_EXPORT_TIMEOUT`** — SDK defaults stand. Stated for clarity; not blocking anything.
- **`OTEL_TRACES_EXPORTER` / `OTEL_METRICS_EXPORTER` / `OTEL_LOGS_EXPORTER`** — we hard-wire `otlp` for all three. The app doesn't bundle alternative exporters. Set to anything other than `otlp` (or unset) and you get our `otlp` defaults; we do not error on this.

### 4.4 Per-signal enablement: not exposed

There is no per-signal on/off toggle in v1. If OTel is enabled, all three signals (traces, metrics, logs) export. Adding `POSTMANPAT_OTEL_TRACES=on|off`-style controls is YAGNI for a single-user tool, and supporting `OTEL_*_EXPORTER=none` would contradict §4.3. If a real "metrics only" requirement appears later, it is an additive change.

---

## 5. Resource attribution (pending)

## 6. Testing strategy (pending)

## 7. Out of scope (pending)
