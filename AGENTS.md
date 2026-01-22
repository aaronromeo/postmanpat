# Repository Guidelines

## Project Structure & Module Organization
- `context/` contains the current project overview and operating constraints.
- Source code, tests, and assets are not yet defined; document new top-level directories as they are introduced (for example `cmd/`, `internal/`, `pkg/`, `tests/`).

## Build, Test, and Development Commands
- **TBD**: Build, test, and local run commands will be defined once the Go module layout and tooling are finalized.
- **CI**: This repository will use GitHub Actions to run build/test workflows on every pull request.

## Coding Style & Naming Conventions
- Go formatting should follow `gofmt` and standard Go naming conventions.
- Keep package names short and lowercase; exported identifiers should use PascalCase.
- When adding linting/formatting tools, document them here.

## Testing Guidelines
- **TBD**: Define test framework(s), coverage expectations, and naming conventions once tests are added.
- Prefer deterministic tests for IMAP interactions; consider local fixtures or mocks instead of live servers.

## Commit & Pull Request Guidelines
- **Commits**: No history to infer conventions yet. Use imperative, scoped messages (for example `email: handle duplicates`).
- **PRs**: Include a concise description, test evidence (commands or CI link), and any operational notes.

## Roadmap / Stages (MVP Focus)
- Stage 1: Functional IMAP ingestion and basic archival to DigitalOcean Spaces.
- Stage 2: Scalable cleanup rules to triage large inboxes (bulk actions, safe defaults).
- Stage 3: Assisted manual review tooling for selective cleanup to reach inbox zero.
- Stage 4: Migration tooling to move mail from Gmail to Fastmail after cleanup.

## Stage 1 Requirements (Cleanup CLI)
- CLI: `postmanpat cleanup` runs on a schedule (cron) with a YAML config file.
- Config includes IMAP credentials, DO Spaces credentials, rules, and reporting.
- Scope: process all IMAP folders (Gmail labels treated as folders).
- Rules: ordered, apply-all; each rule must include folder selection criteria.
- Matchers: age, sender/domain, recipient, body regex, and folder.
- Actions: archive to Spaces, move to folder, delete; action order is defined per rule.
- Archive format: store `.eml` plus decoded `.txt`/`.html` and attachments.
- Archive path: rule-defined template path with variables.
- Attachments: `<message-id>/<attachment-name>.<ext>`; email as `<message-id>.eml`.
- Dedupe: hash of raw `.eml` across folders per run.
- Checkpoint: per-folder last UID in a separate local file; allow reset/ignore.
- Reporting: per-rule stats and errors via Slack or Discord webhook.

## IMAP Callback / Watcher (Realtime)
- Goal: IMAP "callback" equivalent using a long-lived IMAP session with IDLE to trigger rule processing on new inbox mail.
- Scope: single mailbox (INBOX) only for the watcher; no multi-mailbox support needed initially.
- Transport: use IDLE only (no polling fallback) for initial implementation.
- Reliability: handle reconnects with backoff and resume from last UID to avoid missing messages.

## Deployment (DigitalOcean)
- Target deployment is a Docker container on DigitalOcean.
- Decide on hosting form factor (App Platform vs. Droplet) and document environment configuration.

## Agent-Specific Instructions
- Work in small, discussed steps; requirements and stages should be agreed before implementation.
- Keep this document updated as new directories, commands, and workflows are introduced.
