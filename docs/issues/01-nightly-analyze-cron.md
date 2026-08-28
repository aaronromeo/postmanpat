# 01 — Nightly analyze on cron

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0004.
> **Spec:** `docs/superpowers/specs/2026-08-17-nightly-analyze-cron-design.md`

**What to build:** `analyze` runs daily on the docker cron (offset from the hourly cleanup tick) and writes its Analyze Report(s) to a deterministic location on a mounted host directory, overwriting the previous run's report. The scheduled scan covers only recent mail: `age_window.max: 36h` in the live analyze config, and the cron invocation passes `--min-count 1` (a once-a-day sender has count 1 inside a 36h window). rulesgen stays out of the mailbox entirely — this ticket is the whole data pipeline into it.

**Blocked by:** None — can start immediately.

**Status:** code complete (commits `2c3bef6`, `5190b3f`); deployed 2026-08-23 but **analysis did not run** — deploy verification found the analyze cron line absent (`POSTMANPAT_ANALYZE_CONFIG` unset in `.env`) and a compose bug (no volume mount for the analyze config; fixed post-verification). Pending: `.env` + config edits, then re-deploy.

- [x] `analyze` gains an `--out` flag writing report(s) to a deterministic path (one file per analyze rule; overwrite per run)
- [x] Writes are atomic (temp file in target dir, fsync, rename); slug collisions fail fast before any IMAP scan
- [x] Cron line added to `docker/entrypoint.sh`: daily analyze at 03:30 (offset from the hourly cleanup tick), uses the fixed in-container config path `/config/config_analyze.yaml`, output to `/analyze-out`, present only when `POSTMANPAT_ANALYZE_CONFIG` is set (byte-identical crontab when unset)
- [x] Cron invocation passes `--min-count 1`
- [x] `docker-compose.yml` passes `POSTMANPAT_ANALYZE_CONFIG` through; mounts the analyze config read-only at `/config/config_analyze.yaml` (host path from the env var, mirroring the cleanup config mount) and `${POSTMANPAT_ANALYZE_OUT:-./analyze-out}` at `/analyze-out`
- [ ] Deploy step (not code): set `POSTMANPAT_ANALYZE_CONFIG=/opt/docker/rocketman/postmanpat-config/analysis.yml` in the live `/opt/docker/postmanpat/.env` (report dir already mounts at `/opt/docker/postmanpat/analyze-out` by default; no `POSTMANPAT_ANALYZE_OUT` needed)
- [ ] Deploy step (not code): change `age_window.max` from `7d` to `36h` in the live `/opt/docker/rocketman/postmanpat-config/analysis.yml`
- [x] README/AGENTS.md updated (cron is no longer cleanup-only; `--out` and scheduled analyze documented)
- [x] Tests: e2e at the analyze command seam (ftest in-memory IMAP) — deterministic per-rule files, slug collision before scan, overwrite-not-accumulate, missing-directory creation, unset-`--out` temp-file preservation; `go test ./...` green
- [ ] Verified: after a night on the server, a fresh report file exists at the expected path
