# Repository Guidelines

## Overview
- PostmanPat is a single-user Go CLI (module `github.com/aaronromeo/postmanpat`) for IMAP email cleanup/archival, deployed as a Docker container on DigitalOcean.
- Go 1.25.5 (pinned in `.tool-versions`, `go.mod`, and CI). IMAP via `github.com/emersion/go-imap/v2` (beta); tests use `stretchr/testify`.

## Project Structure
- `cli/` — Cobra commands `cleanup`/`watch`/`analyze`; these files hold real orchestration logic (env loading, action dispatch, watch loop), not just flag wiring.
- `appconfig/` — YAML config models + validation; the `ServerMatchers` vs `ClientMatchers` split and the `ignore:` schema live here.
- `imap/` — vendor-neutral IMAP facade; implementation is hidden in `imap/internal/` (`actions`, `searches`, `selectors`, `sessionmgr`, `maildata`).
- `serverrunner/` / `watchrunner/` — cleanup / IDLE-watch orchestration; client-side matchers in `watchrunner/internal/matchers`.
- `ftest/` — integration tests against an in-memory TLS IMAP server; no external services required.
- `bin/` — Python helper scripts (rule generator, config converter) with their own unittest suite; doubles as the Go build output dir (`bin/postmanpat` is gitignored).
- `context/` — project brief and operating constraints; `docs/adr/` — architecture decisions; `docs/prompts/` — prompts requesting specs (input to the spec workflow, not specs); `docs/superpowers/specs/` — approved design specs.

## Build, Test, and Verify
- Build: `go build -o bin ./...` → binary at `bin/postmanpat`.
- Test: `go test ./...` — the only CI step (`.github/workflows/ci.yml`, runs on every push). `ftest/` integration tests run by default (no build tags/skips).
- Single package: `go test ./ftest/` (or `./appconfig/`, `./watchrunner/...`).
- Python suite (NOT in CI — easy to miss): `cd bin && python3 -m unittest discover -p "test_*.py"`; requires PyYAML (`bin/requirements.txt`).
- No Makefile or linter config in the repo; use `gofmt -l .` and `go vet ./...` for verification.

## CLI Behavior
- Config path: `--config` flag or `POSTMANPAT_CONFIG` env. All subcommands auto-load a local `.env` (godotenv) when present.
- `cleanup` (`--dry-run`): server-side matchers only — rules with `client` matchers are rejected. Actions supported at runtime: `delete`, `move` only. Non-idempotent (deletes/moves mail), so validate with `--dry-run`.
- `watch` (`--verbose`, `--test "Rule"` `--limit` `--mailbox`): long-lived IDLE loop on a single mailbox (INBOX). Client-side matchers only — rules with `server` matchers are rejected. Reloads config every 5 minutes (`cli/watch.go`); reconnects after benign IDLE errors and resumes from last UID.
- `analyze` (`--top` `--examples` `--min-count` `--no-ignore`): scans via server matchers and writes a JSON report to a temp file, printing its path. An optional top-level `ignore:` section (`watch:`/`cleanup:` sub-lists) filters Fully Decided messages (on both lists) out of the report; each surviving cluster carries a `suppressed` annotation (`["watch"]`, `["cleanup"]`, or both) that the rule generator uses to skip prompts. See `CONTEXT.md` and `docs/adr/0002-suppression-via-report-annotation.md`.
- `age_window` uses IMAP INTERNALDATE (SINCE/BEFORE criteria), not the `Date:` header.

## Environment Variables
- IMAP: `POSTMANPAT_IMAP_HOST/PORT/USER/PASS`; S3/Spaces: `POSTMANPAT_S3_ENDPOINT/REGION/BUCKET/KEY/SECRET`; reporting: `POSTMANPAT_WEBHOOK_URL`.
- Real per-environment configs (`config/config_cleanup.yaml`, `config/config_watch.yaml`, `config/config_analyze.yaml`) are gitignored.

## Gotchas
- `config/config_example.yaml` and parts of `README.md` are aspirational: the `archive` action type and `archive.path_template` are NOT implemented (runtime errors with "unsupported action type"; only `delete`/`move` constants exist in `appconfig`), there is no `.env.sample`, and the Docker cron runs hourly (`docker/entrypoint.sh`) not every 15 minutes as the README claims.
- `docker-compose.yml` builds an `announcements` image from `./announcements`. It is declared in `.gitmodules` but has no gitlink in HEAD, so `git submodule update --init` will NOT populate it — clone `git@github.com:aaronromeo/announcements.git` into `./announcements` before `docker compose up --build`.
- `repomix-output.xml` is a generated repo dump; do not edit.

## OTel Status
- Implemented: `obs/` package (Init, env config, resource, `WrapCleanupRunner`, `WrapWatchRunner`), trace instrumentation for `watch` (cycle/message/rule_evaluated/action) and `cleanup` (invocation/rule/action + per-message events), IMAP RED metrics, OTLP gRPC export (plaintext via `OTEL_EXPORTER_OTLP_INSECURE` or `http://` scheme), docker-compose + cron OTLP env wiring.
- Deferred: slog→OTel logs bridge (see `docs/superpowers/specs/2026-06-13-otel-foundation-cleanup-design.md` §3.3), OTLP/HTTP exporter.

## Operating Constraints
- `context/roles_and_constraints.md` defines the working mode: Plan/Act split (start in Plan, share a plan before edits, only make changes on an explicit `ACT`), and ask permission before running state-changing commands.
- Keep this document updated as new directories, commands, and workflows are introduced.
- IMAP logic must remain vendor-neutral; avoid provider-specific assumptions or headers unless explicitly documented and optional.
