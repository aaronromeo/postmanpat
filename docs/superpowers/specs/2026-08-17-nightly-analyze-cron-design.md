# Nightly Scheduled Analyze

Delivery stage 01 of the rulesgen effort (stub: `docs/issues/01-nightly-analyze-cron.md`; architecture: `docs/adr/0004-scheduled-analyze-file-ingestion.md`; vocabulary: `CONTEXT.md`).

## Problem Statement

Today `analyze` runs only when I remember to run it: a manual `docker compose run` with a `TMPDIR` mount so the report survives container teardown, then a manual session with the rule generator. The report lands at a random temp path printed to stdout, so nothing downstream can find it programmatically. With rulesgen coming — a persistent Review Queue fed by scheduled Analyze Reports — I need the scan to happen every night without me, landing at a predictable place, scoped to recent mail so the report describes what is *newly* undecided rather than my whole mailbox history.

## Solution

Make `analyze` a scheduled citizen of the existing docker cron alongside `cleanup`, and teach it to write to a deterministic output location:

- A new `--out` flag on `analyze` writes each rule's report to a predictable per-rule file in a mounted directory, **overwriting** the previous run's file (ADR 0004: reports are ephemeral inputs; the decision store is the memory). Without `--out`, the current temp-file behavior is unchanged.
- The container's cron gains a **daily** analyze line (offset from the hourly cleanup tick) that passes `--min-count 1` — inside a 36-hour sliding window a once-a-day sender has count 1, and the default of 2 would hide exactly the mail the Review Queue exists to surface.
- The cron line exists only when an analyze config is provided to the container, so deployments without one see no nightly failure.
- The 36-hour window itself is a **live-config edit** (`age_window.max: 36h` on the analyze rules), not code — the schema already supports it.

rulesgen stays out of the mailbox entirely: this cron job is the whole data pipeline into it.

## User Stories

1. As the operator, I want analyze to run every night without manual intervention, so that undecided mail is continuously surfaced for rule generation.
2. As the operator, I want the scheduled run to write to a deterministic known path, so that downstream ingestion can find reports without parsing stdout or globbing temp directories.
3. As the operator, I want each run to overwrite the previous report for the same rule, so that there is exactly one latest report per rule and disk usage stays flat.
4. As the operator, I want one report file per analyze rule with a filename predictable from the rule name, so that multi-rule configs never clobber each other.
5. As the operator, I want colliding derived filenames to fail the run loudly *before* any mailbox scanning, so that I never silently lose one rule's report under another's.
6. As the operator, I want report writes to be atomic, so that no reader ever ingests a half-written JSON file.
7. As the operator, I want the scheduled invocation to pass `--min-count 1`, so that once-a-day senders (count 1 inside the window) still produce clusters.
8. As the operator, I want the analyze tick scheduled away from the hourly cleanup tick, so the two commands do not routinely open IMAP sessions at the same moment.
9. As the operator, I want the analyze cron line present only when I have provided an analyze config, so that a deployment without one stays silent rather than failing nightly.
10. As the operator, I want the existing one-off `TMPDIR` workflow to keep working unchanged, so that ad-hoc deep scans and `--no-ignore` audits remain possible.
11. As the operator, I want each written report path still printed to stdout, so the cron logs show exactly what was produced.
12. As the operator, I want the scheduled report to carry identical semantics to a manual one — same schema, same ignore filtering and Suppressed annotations — so that both the Python generator and rulesgen consume it unchanged.
13. As the operator, I want the deploy-time steps (env var, host directory, live-config window edit) written down, so the change survives my future re-provisioning.
14. As the operator, I want README and AGENTS.md to reflect that the container's cron is no longer cleanup-only, so the deployment's behavior is discoverable.

## Implementation Decisions

- **New `--out` flag (analyze command).** A directory. When set, every rule's report is written into it at a deterministic per-rule filename; when unset, behavior is exactly as today (temp file, path printed). Report contents and schema are identical on both paths — the flag changes only *where* the bytes land. Each written path is still printed to stdout.
- **Deterministic per-rule filenames.** The filename derives from the rule's name, sanitized to a lowercase slug (runs of non-alphanumeric characters collapse to a dash). Uniqueness is preflighted right after config validation: if two rules sanitize to the same slug, the command errors out naming both rules before any IMAP session is opened. One report per rule remains the model; the mailbox scanned stays the first entry of the rule's `server.folders` (existing behavior, unchanged).
- **Atomic overwrite.** Reports are written to a temp file in the target directory, synced, then renamed over the previous file — a reader never sees a partial report. The target directory is created if missing (a container mount point is expected to exist, but a typo'd path failing loudly at write time is still better than a late error).
- **Cron wiring (docker/entrypoint.sh).** A new optional environment variable, `POSTMANPAT_ANALYZE_CONFIG`, carries the **host** path of the analyze config, mirroring `POSTMANPAT_CLEANUP_CONFIG`. Compose mounts that file read-only at the fixed in-container path `/config/config_analyze.yaml`, which the cron line references. When the variable is set, the entrypoint writes a second cron line running analyze **daily at 03:30** (the hourly cleanup tick sits at minute 0) with `--out` pointing at the mounted report directory and `--min-count 1`. When unset, no analyze line is emitted — the crontab is byte-identical to today. The schedule is hardcoded, matching how the cleanup schedule is hardcoded.
- **Compose wiring (docker-compose.yml).** Two mounts: the analyze config file (`${POSTMANPAT_ANALYZE_CONFIG:-./config/config_analyze.yaml}` → `/config/config_analyze.yaml`, read-only, mirroring the cleanup config mount) and a host directory at the `--out` location so reports survive the container and are reachable by the future rulesgen service.
- **No Go changes outside the analyze command.** `appconfig` already parses `age_window` (`36h` is a valid value), and the ignore/suppression pipeline needs no extension. The Python generator is untouched.
- **Deploy steps (documentation, not code).** Add `age_window.max: 36h` to each rule in the live analyze config; add `POSTMANPAT_ANALYZE_CONFIG` to the live `.env`; ensure the host report directory exists (the `ls`-guard habit from the one-off docs applies).
- **Docs.** README gains a short "scheduled analyze" subsection alongside the one-off flow; AGENTS.md's CLI Behavior and cron claims are updated (cron is no longer cleanup-only).

## Testing Decisions

Good tests here verify external behavior only: files appearing (or not) at expected paths with expected contents, commands failing loudly on bad input. No mocks — the codebase's convention is real in-memory IMAP servers. **One seam, and it already exists:** the `analyze` cobra command driven end-to-end through the root command against the in-memory TLS IMAP server (prior art: the analyze ignore e2e test).

- **`--out` set:** reports land at the deterministic per-rule paths in the given directory; report JSON parses and matches expectations (account for the two seeded INBOX messages every ftest server starts with); a second run overwrites rather than accumulates (file count unchanged, fresh content).
- **`--out` unset:** the printed temp path exists and contains the report (pins the unchanged one-off behavior).
- **Slug collision:** a config whose two rule names sanitize to the same slug fails with an error naming both rules, and no scan occurs.
- **Preflight ordering:** the collision error surfaces without any IMAP connection (assertable by pointing the test at an unreachable server address and still getting the slug error).
- **Not automated:** the entrypoint shell changes. The repo has no shell test harness and this spec introduces none; the cron line is verified by inspection and by the deploy-time check (a report file exists at the expected path the next morning). The Python helper suite is unaffected.

## Out of Scope

- **rulesgen itself** — ingestion, the store, the Review Queue, and everything in delivery stages 02–05 (see `docs/issues/`).
- **Report history/retention** — timestamped accumulation was considered and rejected (ADR 0004); overwrite is the design.
- **The live-config edit** (`age_window.max: 36h`) — documented here as a deploy step; the gitignored config itself is not committed.
- **A configurable cron schedule** — hardcoded like cleanup's; revisit only if a second cadence is ever wanted.
- **OTel instrumentation of analyze** — the existing observability work covers watch and cleanup only; extending it is a separate decision.
- **Publishing this spec to GitHub Issues** — deferred (GitHub outage); the spec lives in the repo and the stub in `docs/issues/01-nightly-analyze-cron.md` links here.

## Further Notes

- This is the smallest shippable stage and is deliberately independent of stage 02 (ingestion can be developed against reports from the existing one-off workflow, so the two may proceed in parallel).
- Once this lands, the manual `TMPDIR` flow remains the escape hatch for deep scans: run one-off with a wide or absent window and the default `--min-count 2` whenever a full-mailbox audit is wanted.
