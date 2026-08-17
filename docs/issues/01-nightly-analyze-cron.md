# 01 — Nightly analyze on cron

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0004.
> **Spec:** `docs/superpowers/specs/2026-08-17-nightly-analyze-cron-design.md`

**What to build:** `analyze` runs daily on the docker cron (offset from the hourly cleanup tick) and writes its Analyze Report(s) to a deterministic location on a mounted host directory, overwriting the previous run's report. The scheduled scan covers only recent mail: `age_window.max: 36h` in the live analyze config, and the cron invocation passes `--min-count 1` (a once-a-day sender has count 1 inside a 36h window). rulesgen stays out of the mailbox entirely — this ticket is the whole data pipeline into it.

**Blocked by:** None — can start immediately.

**Status:** stub

- [ ] `analyze` gains an `--out` flag writing report(s) to a deterministic path (one file per analyze rule; overwrite per run)
- [ ] Cron line added to `docker/entrypoint.sh`: daily analyze, off-hours from the cleanup tick, output to the mounted analyze-out dir
- [ ] Cron invocation passes `--min-count 1`
- [ ] Deploy step (not code): add `age_window.max: 36h` to the live `config_analyze.yaml`
- [ ] README/AGENTS.md updated (cron is no longer cleanup-only; analyze cadence documented)
- [ ] Verified: after a night on the server, a fresh report file exists at the expected path
