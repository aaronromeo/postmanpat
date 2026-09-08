# Repository Guidelines

## Overview
- PostmanPat is a single-user Go CLI (module `github.com/aaronromeo/postmanpat`) for IMAP email cleanup/archival, deployed as a Docker container on DigitalOcean.
- Go 1.25.5 (pinned in `.tool-versions`, `go.mod`, and CI). IMAP via `github.com/emersion/go-imap/v2` (beta); tests use `stretchr/testify`.

## Project Structure
- `cli/` — Cobra commands `cleanup`/`watch`/`analyze`; these files hold real orchestration logic (env loading, action dispatch, watch loop), not just flag wiring.
- `appconfig/` — YAML config models + validation; the `ServerMatchers` vs `ClientMatchers` split and the `ignore:` schema live here.
- `imap/` — vendor-neutral IMAP facade; implementation is hidden in `imap/internal/` (`actions`, `searches`, `selectors`, `sessionmgr`, `maildata`).
- `serverrunner/` / `watchrunner/` — cleanup / IDLE-watch orchestration; client-side matchers in `watchrunner/internal/matchers`.
- `rulesgen/` — Review Queue web service (SQLite decision store, report-file ingestion, read-only queue page); cobra wiring in `cli/rulesgen.go`, compose service `postmanpat-rulesgen`.
- `ftest/` — integration tests against an in-memory TLS IMAP server; no external services required.
- `bin/` — Python helper scripts (rule generator, config converter) with their own unittest suite; doubles as the Go build output dir (`bin/postmanpat` is gitignored). `postmanpat-generate-rules.py` splits cleanup rules with multiple recipient aliases one-rule-per-alias, because server matchers AND multiple `recipients` values together (a multi-alias rule can never match).
- `context/` — project brief and operating constraints; `docs/adr/` — architecture decisions; `docs/prompts/` — prompts requesting specs (input to the spec workflow, not specs); `docs/superpowers/specs/` — approved design specs.

## Build, Test, and Verify
- Build: `go build -o bin ./...` → binary at `bin/postmanpat`.
- Test: `go test ./...` — the only CI step (`.github/workflows/ci.yml`, runs on every push). `ftest/` integration tests run by default (no build tags/skips).
- Test quirks: `ftest.SetupIMAPServer` always seeds two built-in INBOX messages (News/Other from example.com/example.org) in addition to your fixtures — account for them in expected match counts. `cli/cleanup_trace_test.go` exercises `runCleanupLogic`, a hand-duplicated copy of the real `runCleanup` in `cli/cleanup.go` — it does NOT cover the production path.
- Single package: `go test ./ftest/` (or `./appconfig/`, `./watchrunner/...`).
- Python suite (NOT in CI — easy to miss): `cd bin && python3 -m unittest discover -p "test_*.py"`; requires PyYAML (`bin/requirements.txt`).
- No Makefile or linter config in the repo; use `gofmt -l .` and `go vet ./...` for verification.

## CLI Behavior
- Config path: `--config` flag or `POSTMANPAT_CONFIG` env. All subcommands auto-load a local `.env` (godotenv) when present.
- `cleanup` (`--dry-run`): processes rules in order, applies all actions, and ABORTS the whole run on the first error (e.g. one bad folder name kills the cron run). Server-side matchers only — rules with `client` matchers are rejected. Actions supported at runtime: `delete`, `move` only. Non-idempotent (deletes/moves mail), so validate with `--dry-run`. Stdout shows `config summary` at start, per-rule lines only in dry-run, and a final `cleanup complete` (`rules_matched`/`messages_matched`/`dry_run`) — live runs otherwise log nothing.
- Matcher semantics trap: server matchers AND multiple values within one field (`combineAnd` in `imap/internal/searches/manager.go`) — `recipients: [a, b]` requires BOTH in To/Cc and can never match normal mail; use one alias per rule. Client matchers (watch) OR them (`matchAnyRegex`). `appconfig.Load` is non-strict: a typo'd matcher key (e.g. `mailedby_substring`) is silently dropped.
- `watch` (`--verbose`, `--test "Rule"` `--limit` `--mailbox`): long-lived IDLE loop on a single mailbox (INBOX). Client-side matchers only — rules with `server` matchers are rejected. Reloads config every 5 minutes (`cli/watch.go`); reconnects after benign IDLE errors and resumes from last UID.
- `analyze` (`--top` `--examples` `--min-count` `--no-ignore` `--out`): scans via server matchers and writes one JSON report per rule. Without `--out` it writes a temp file and prints the path; with `--out <dir>` it writes deterministic per-rule files (`postmanpat-analyze-<slug>.json`, overwritten each run, atomic temp+rename) and preflight-errors on slug collisions before any IMAP scan. An optional top-level `ignore:` section (`watch:`/`cleanup:` sub-lists) filters Fully Decided messages (on both lists) out of the report; each surviving cluster carries a `suppressed` annotation (`["watch"]`, `["cleanup"]`, or both) that the rule generator uses to skip prompts. See `CONTEXT.md` and `docs/adr/0002-suppression-via-report-annotation.md`.
- `age_window` uses IMAP INTERNALDATE (SINCE/BEFORE criteria), not the `Date:` header.
- `rulesgen serve` (`--addr` `--reports` `--db` `--poll`): read-only Review Queue web service (default port 8092). Ingests `postmanpat-analyze-*.json` from the reports dir into a SQLite store at startup and every `--poll`; never opens IMAP. Pending clusters dedupe by cluster ID, persist across reports with `last_seen`, refresh count/examples on re-ingestion; `template_lens` never ingested; suppressed-for-both clusters hidden from the queue page (half-suppressed shown with badges). `GET /healthz` for liveness. Decision capture is a later stage — no write routes exist yet. See `CONTEXT.md` (rulesgen, Review Queue, Cluster Decision) and ADRs 0003/0004.

## Environment Variables
- IMAP: `POSTMANPAT_IMAP_HOST/PORT/USER/PASS`; S3/Spaces: `POSTMANPAT_S3_ENDPOINT/REGION/BUCKET/KEY/SECRET`; reporting: `POSTMANPAT_WEBHOOK_URL`.
- Real per-environment configs (`config/config_cleanup.yaml`, `config/config_watch.yaml`, `config/config_analyze.yaml`) are gitignored. The live deployment mounts configs from `/opt/docker/rocketman/postmanpat-config/` (via `POSTMANPAT_CLEANUP_CONFIG`/`POSTMANPAT_WATCH_CONFIG`/`POSTMANPAT_ANALYZE_CONFIG` in `.env`); edits there take effect on the next cron tick with no restart.
- Validate config changes against the real mailbox read-only before trusting them: `docker compose run --rm -v <host-cfg>:/config/config_cleanup.yaml:ro postmanpat postmanpat cleanup --config /config/config_cleanup.yaml --dry-run` (rules that match log per-rule counts; silent rules matched zero).

## Gotchas
- `config/config_example.yaml` and parts of `README.md` are aspirational: the `archive` action type and `archive.path_template` are NOT implemented (runtime errors with "unsupported action type"; only `delete`/`move` constants exist in `appconfig`), there is no `.env.sample`, and the Docker cron runs `cleanup` hourly (`docker/entrypoint.sh`) not every 15 minutes as the README claims. When `POSTMANPAT_ANALYZE_CONFIG` is set, the same cron also runs `analyze` daily at 03:30 with `--out /analyze-out --min-count 1`; without it the crontab is cleanup-only.
- `docker-compose.yml` builds an `announcements` image from `./announcements`. It is declared in `.gitmodules` but has no gitlink in HEAD, so `git submodule update --init` will NOT populate it — clone `git@github.com:aaronromeo/announcements.git` into `./announcements` before `docker compose up --build`.
- `repomix-output.xml` is a generated repo dump; do not edit.

## OTel Status
- Implemented: `obs/` package (Init, env config, resource, `WrapCleanupRunner`, `WrapWatchRunner`), trace instrumentation for `watch` (cycle/message/rule_evaluated/action) and `cleanup` (invocation/rule/action + per-message events), IMAP RED metrics, OTLP gRPC export (plaintext via `OTEL_EXPORTER_OTLP_INSECURE` or `http://` scheme), docker-compose + cron OTLP env wiring.
- Deferred: slog→OTel logs bridge (see `docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md` §3.3), OTLP/HTTP exporter.

## Operating Constraints
- `context/roles_and_constraints.md` defines the working mode: Plan/Act split (start in Plan, share a plan before edits, only make changes on an explicit `ACT`), and ask permission before running state-changing commands.
- Keep this document updated as new directories, commands, and workflows are introduced.
- IMAP logic must remain vendor-neutral; avoid provider-specific assumptions or headers unless explicitly documented and optional.
