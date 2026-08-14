# Repository Guidelines

## Overview
- PostmanPat is a single-user Go CLI (module `github.com/aaronromeo/postmanpat`) for IMAP email cleanup/archival, deployed as a Docker container on DigitalOcean.
- Go 1.25.5 (pinned in `.tool-versions` and `go.mod`). IMAP via `github.com/emersion/go-imap/v2` (beta); tests use `stretchr/testify`.

## Project Structure
- `cmd/postmanpat/` — thin entrypoint; calls `cli.Execute()`.
- `cli/` — Cobra command wiring: `root`, `cleanup`, `watch`, `analyze` (+ tests).
- `appconfig/` — YAML config models, load/validate, env helpers.
- `imap/` — vendor-neutral IMAP facade; `internal/` holds implementation (`actions`, `searches`, `selectors`, `sessionmgr`, `maildata`).
- `serverrunner/` — cleanup orchestration; defines the `ServerRunner` interface.
- `watchrunner/` — IDLE-watch orchestration; `internal/matchers` holds client-side matchers.
- `announcer/` — webhook (Discord/Slack) reporting client.
- `ftest/` — integration tests using an in-memory TLS IMAP server; no external services required.
- `bin/` — Python helper scripts (require PyYAML) for converting watch configs to cleanup configs and generating rules. `postmanpat-generate-rules.py` accepts `--ignore-out` for authoring ignore entries and reads the report's `suppressed` annotation (no config-side matching). Cleanup rules with multiple recipient aliases are split one-rule-per-alias, because server matchers AND multiple `recipients` values together (a multi-alias rule can never match). Python tests (`test_*.py`) run via stdlib unittest — `python3 -m unittest discover -s bin` (pytest is NOT installed; not part of CI).
- `context/` — project brief and operating constraints (`overview.md`, `requirements_stage1.md`, `roles_and_constraints.md`).
- `docs/prompts/` — authored prompts requesting design specs; a prompt here is the input to the spec-writing workflow, not a spec itself.
- `docs/superpowers/specs/` — approved design specs (see OTel status below).

## Build, Test, and Verify
- Build: `go build -o bin ./...` → binary at `bin/postmanpat`.
- Test: `go test ./...` — 15 packages; this is the only CI step (`.github/workflows/ci.yml`, runs on every push). Integration tests in `ftest/` run by default (no build tags/skips).
- Test quirks: `ftest.SetupIMAPServer` always seeds two built-in INBOX messages (News/Other from example.com/example.org) in addition to your fixtures — account for them in expected match counts. `cli/cleanup_trace_test.go` exercises `runCleanupLogic`, a hand-duplicated copy of the real `runCleanup` in `cli/cleanup.go` — it does NOT cover the production path.
- Single package: `go test ./ftest/` (or `./appconfig/`, `./watchrunner/...`).
- No Makefile or linter config in the repo; use `gofmt -l .` and `go vet ./...` for verification.

## CLI Behavior
- Config path: `--config` flag or `POSTMANPAT_CONFIG` env. A local `.env` file is auto-loaded (godotenv) when present.
- `cleanup` (`--dry-run`): processes rules in order, applies all actions, and ABORTS the whole run on the first error (e.g. one bad folder name kills the cron run). Server-side matchers only — rules with `client` matchers are rejected. Actions supported at runtime: `delete`, `move` only. Non-idempotent (deletes/moves mail), so use `--dry-run` for validation. Stdout shows `config summary` at start, per-rule lines only in dry-run, and a final `cleanup complete` (`rules_matched`/`messages_matched`/`dry_run`) — live runs otherwise log nothing.
- Matcher semantics trap: server matchers AND multiple values within one field (`combineAnd` in `imap/internal/searches/manager.go`) — `recipients: [a, b]` requires BOTH in To/Cc and can never match normal mail; use one alias per rule. Client matchers (watch) OR them (`matchAnyRegex`). `appconfig.Load` is non-strict: a typo'd matcher key (e.g. `mailedby_substring`) is silently dropped.
- `watch` (`--verbose`, `--test "Rule"` `--limit` `--mailbox`): long-lived IDLE loop on a single mailbox (INBOX). Client-side matchers only — rules with `server` matchers are rejected. Reloads config every 5 minutes; reconnects after benign IDLE errors and resumes from last UID.
- `analyze` (`--top` `--examples` `--min-count` `--no-ignore`): scans via server matchers and writes a JSON report to a temp file, printing its path. An optional top-level `ignore:` section (`watch:`/`cleanup:` sub-lists) filters Fully Decided messages (on both lists) out of the report; each surviving cluster carries a `suppressed` annotation (`["watch"]`, `["cleanup"]`, or both) that the rule generator uses to skip prompts. See `CONTEXT.md` and `docs/adr/0002-suppression-via-report-annotation.md`.
- `age_window` uses IMAP INTERNALDATE, not the `Date:` header.

## Environment Variables
- IMAP: `POSTMANPAT_IMAP_HOST/PORT/USER/PASS`; S3/Spaces: `POSTMANPAT_S3_ENDPOINT/REGION/BUCKET/KEY/SECRET`; reporting: `POSTMANPAT_WEBHOOK_URL`.
- Real per-environment configs (`config/config_cleanup.yaml`, `config/config_watch.yaml`, `config/config_analyze.yaml`) are gitignored. The live deployment mounts configs from `/opt/docker/rocketman/postmanpat-config/` (via `POSTMANPAT_CLEANUP_CONFIG`/`POSTMANPAT_WATCH_CONFIG` in `.env`); edits there take effect on the next cron tick with no restart.
- Validate config changes against the real mailbox read-only before trusting them: `docker compose run --rm -v <host-cfg>:/config/config_cleanup.yaml:ro postmanpat postmanpat cleanup --config /config/config_cleanup.yaml --dry-run` (rules that match log per-rule counts; silent rules matched zero).

## Gotchas
- `config/config_example.yaml` and parts of `README.md` are aspirational: the `archive` action type and `archive.path_template` are NOT implemented (runtime errors with "unsupported action type"), there is no `.env.sample`, and the Docker cron runs hourly (`docker/entrypoint.sh`) not every 15 minutes as the README claims.
- `docker-compose.yml` builds an `announcements` git submodule — initialize it (`git submodule update --init --recursive`) before `docker compose up --build`.
- `repomix-output.xml` is a generated repo dump; do not edit.

## OTel Status
- Implemented: `obs/` package (Init, env config, resource, `WrapCleanupRunner`, `WrapWatchRunner`), trace instrumentation for `watch` (cycle/message/rule_evaluated/action) and `cleanup` (invocation/rule/action + per-message events), IMAP RED metrics, OTLP gRPC export (plaintext via `OTEL_EXPORTER_OTLP_INSECURE` or `http://` scheme), docker-compose + cron OTLP env wiring.
- Deferred: slog→OTel logs bridge (see `docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md` §3.3), OTLP/HTTP exporter.

## Operating Constraints
- `context/roles_and_constraints.md` defines the working mode: Plan/Act split (start in Plan, share a plan before edits, only make changes on an explicit `ACT`), and ask permission before running state-changing commands.
- Keep this document updated as new directories, commands, and workflows are introduced.
- IMAP logic must remain vendor-neutral; avoid provider-specific assumptions or headers unless explicitly documented and optional.
