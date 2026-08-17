# Scheduled analyze on cron; rulesgen ingests report files and never touches IMAP

`analyze` runs on the docker cron (daily), like `cleanup`, writing its Analyze Report(s) to a mounted directory at a deterministic `--out` path and overwriting the previous run's report. rulesgen discovers and ingests these files; it never opens an IMAP connection of its own. The scheduled run scopes the scan to recent mail via `age_window.max: 36h` in the analyze config and passes `--min-count 1`, because a once-a-day sender produces a count of 1 inside a 36-hour window.

## Considered Options

- **Service-owned in-process scheduler running the analyze logic directly**: rejected — it would give the UI service live mailbox access (IMAP credentials in its environment, a second client path to operate and secure) for marginal convenience. Cron + files keeps rulesgen read-only versus the mailbox and reuses the existing container scheduling pattern.
- **Service shells out to the `postmanpat analyze` binary per run**: rejected — still requires IMAP credentials in the service's environment and couples UI freshness to a scan the service must supervise.
- **Retain every report (timestamped files)**: rejected for now — reports are ephemeral inputs; the decision store is the memory. Overwriting keeps ingestion trivially deterministic.

## Consequences

- Review Queue semantics follow from the sliding window: a Cluster appears in a report only when mail arrived within it, so Pending Clusters persist across reports with a tracked `last_seen`, and Snoozed means "hidden until a newer report containing the Cluster arrives."
- Report counts mean "messages in the window," not mailbox totals.
- If report history is ever wanted (trend views), retention is a cron/`--out` naming change, not a schema change.
