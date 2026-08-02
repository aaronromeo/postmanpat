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
- `bin/` — Python helper scripts (require PyYAML) for converting watch configs to cleanup configs and generating rules.
- `context/` — project brief and operating constraints (`overview.md`, `requirements_stage1.md`, `roles_and_constraints.md`).
- `docs/superpowers/specs/` — approved design specs (see OTel status below).

## Build, Test, and Verify
- Build: `go build -o bin ./...` → binary at `bin/postmanpat`.
- Test: `go test ./...` — 71 tests across 14 packages; this is the only CI step (`.github/workflows/ci.yml`, runs on every push). Integration tests in `ftest/` run by default (no build tags/skips).
- Single package: `go test ./ftest/` (or `./appconfig/`, `./watchrunner/...`).
- No Makefile or linter config in the repo; use `gofmt -l .` and `go vet ./...` for verification.

## CLI Behavior
- Config path: `--config` flag or `POSTMANPAT_CONFIG` env. A local `.env` file is auto-loaded (godotenv) when present.
- `cleanup` (`--dry-run`): processes rules in order, applies all actions. Server-side matchers only — rules with `client` matchers are rejected. Actions supported at runtime: `delete`, `move` only. Non-idempotent (deletes/moves mail), so use `--dry-run` for validation.
- `watch` (`--verbose`, `--test "Rule"` `--limit` `--mailbox`): long-lived IDLE loop on a single mailbox (INBOX). Client-side matchers only — rules with `server` matchers are rejected. Reloads config every 5 minutes; reconnects after benign IDLE errors and resumes from last UID.
- `analyze` (`--top` `--examples` `--min-count`): scans via server matchers and writes a JSON report to a temp file, printing its path.
- `age_window` uses IMAP INTERNALDATE, not the `Date:` header.

## Environment Variables
- IMAP: `POSTMANPAT_IMAP_HOST/PORT/USER/PASS`; S3/Spaces: `POSTMANPAT_S3_ENDPOINT/REGION/BUCKET/KEY/SECRET`; reporting: `POSTMANPAT_WEBHOOK_URL`.
- Real per-environment configs (`config/config_cleanup.yaml`, `config/config_watch.yaml`, `config/config_analyze.yaml`) are gitignored.

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
