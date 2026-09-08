# rulesgen: Report Ingestion, Decision Store, Read-Only Review Queue

Delivery stage 02 of the rulesgen effort (stub: `docs/issues/02-rulesgen-readonly-queue.md`; architecture: `docs/adr/0003-rulesgen-coexists-with-python-generator.md`, `docs/adr/0004-scheduled-analyze-file-ingestion.md`; vocabulary: `CONTEXT.md`).

## Problem Statement

Nightly Analyze Reports now land at a deterministic mounted path, but working through them still means SSH and the Python generator script, prompted one cluster at a time, with its own file-based checkpoint. Nothing gives me a persistent view of what is undecided: the report is scoped to a 36-hour sliding window, so the moment mail ages out of the window a cluster I never decided on silently stops appearing — and once stage 01's report gets overwritten, there is no memory of it at all. I want to open a private web page and see every Cluster awaiting a Cluster Decision, with fresh counts and when mail last arrived for it — a Review Queue that survives sliding windows and service restarts.

## Solution

A new long-lived command, `postmanpat rulesgen serve`, running as a docker-compose service on the private network (no auth in v1):

- It **ingests** the stage-01 Analyze Report files from the mounted report directory into a **SQLite-backed decision store** — startup, then periodically. It reads report files only and never opens an IMAP connection (ADR 0004); its environment carries no mailbox credentials.
- It renders a **read-only Review Queue**: every Pending Cluster with its Lens, Lens Key values, count, latest date, examples, Suppressed badges, and `last_seen`. Pending Clusters persist across reports (the 36h window hides nothing that lacks a decision), dedupe by cluster ID, and refresh count/examples/`last_seen` whenever a newer report re-contains them. `template_lens` Clusters are never ingested (script parity).
- The store schema anticipates the three rule-type lanes (watch / One-Time Cleanup / Ongoing Cleanup) even though this slice captures no decisions — stage 03 becomes inserts, not a migration.

## User Stories

1. As the operator, I want a persistent Review Queue web page, so that I can see undecided Clusters without SSH and the interactive script.
2. As the operator, I want ingestion to happen automatically at startup and on a timer, so the queue reflects the latest nightly report without any action on my part.
3. As the operator, I want Pending Clusters to persist across reports even after the 36h window slides past their mail, so nothing awaiting a decision silently disappears.
4. As the operator, I want `last_seen` shown per Cluster, so I know how recently mail arrived for it.
5. As the operator, I want a Cluster re-appearing in a newer report to refresh its count/examples, so the queue shows current data, not first-seen data.
6. As the operator, I want Clusters deduplicated by cluster ID across reports, so the queue never lists the same Cluster twice.
7. As the operator, I want `template_lens` Clusters never ingested, so the queue matches exactly what the generator could ever present.
8. As the operator, I want Clusters suppressed for both watch and cleanup to stay out of the queue, so Fully Decided noise never wastes my attention.
9. As the operator, I want half-suppressed Clusters visible with a Suppressed badge naming the blocked lane, so I know which rule types are off the table before deciding.
10. As the operator, I want re-ingesting an unchanged report to change nothing, so restarts and re-scans are free and idempotent.
11. As the operator, I want a corrupt or unreadable report file skipped with a log line while other files still ingest, so one bad file never blanks the queue.
12. As the operator, I want the store on a mounted path that survives container restarts, so the queue is stable across redeploys.
13. As the operator, I want the service attached to the private network with no published ports and no auth (v1), so it inherits the LAN trust boundary exactly like the announcements service.
14. As the operator, I want the store schema to anticipate the three rule-type lanes now, so decision capture in stage 03 needs no migration.
15. As the operator, I want the service to have no IMAP credentials in its environment, so the web surface cannot touch the mailbox even if it is ever compromised.
16. As the operator, I want each queue entry to show Lens, Lens Key values, count, latest date, and examples, so I can decide without opening a mail client.
17. As the operator, I want a health endpoint, so compose healthchecks work the way they do for announcements.
18. As the operator, I want lane offerability per Lens expressed as data, so a future cleanup lane for `recipient_tag_lens` is a table edit, not new branching.
19. As the operator, I want report ingestion to read files only, so the nightly writer (the cron container) is never disturbed.
20. As the operator, I want ingestion to process report files in a deterministic order, so re-ingestion behavior is reproducible.

## Implementation Decisions

- **New `rulesgen` cobra command group with a `serve` subcommand.** Flags: `--addr` (default `:8092`), `--reports` (report directory), `--db` (SQLite file path), `--poll` (re-ingest interval, default `1m`). `serve` opens the store, ingests once, then runs a poller and the HTTP server until shutdown. A new flat `rulesgen` package holds the store, ingestion, and HTTP server; `cli/rulesgen.go` is thin wiring. No facade/internal split — unlike `imap/`, there is no vendor-neutrality boundary to hide.
- **SQLite via `modernc.org/sqlite`** — pure Go, no CGO, so the Docker build stays CGO-free.
- **Store schema.** `clusters`: one row per ingested Cluster, primary key `cluster_id`; columns for lens, Lens Key values (JSON), count, latest date, examples (JSON), signals (JSON), suppressed annotations (JSON), `first_seen`, `last_seen`. Upsert refreshes every mutable column; `first_seen` is set on insert only. `decisions`: primary key (`cluster_id`, `lane`) with `lane` in {watch, one-time cleanup, ongoing cleanup}, `decision` in {generated, declined, ignored, snoozed}, `decided_at`, and a nullable payload for stage-03 rule fragments. The table is created empty in this slice — no decision is captured yet, but the shape is fixed now so later stages are INSERT-only.
- **Ingestion semantics.** Discover files matching the stage-01 naming convention (`postmanpat-analyze-*.json`) in the reports directory, sorted by name (atomic-write temp files are dot-prefixed and never match). For each file: parse, walk `list_lens`, `sender_unsub_lens`, and `recipient_tag_lens` (never `template_lens`), and upsert each Cluster with `last_seen` taken from the report's `generated_at`. A file that fails to parse is skipped with a log line; ingestion of the remaining files continues (the nightly cron overwrites the bad file anyway). An empty or missing directory yields an empty queue, not an error — the first night's report simply has not arrived yet.
- **Queue page (read-only).** `/` renders every Cluster with no decision in any lane that is not suppressed for both watch and cleanup, ordered by `last_seen` descending: Lens, Lens Key values, count, latest date, examples, Suppressed badges, first/last seen. No forms and no POST routes exist anywhere in v1. `/healthz` returns 200 for the compose healthcheck.
- **Lane offerability as data.** A per-Lens map: `list_lens` and `sender_unsub_lens` offer watch, one-time cleanup, and ongoing cleanup; `recipient_tag_lens` offers watch only (script parity — the script refuses cleanup for recipient tags; if the recipient-tag server-side cleanup prompt lands, enabling it is a map edit).
- **Compose service.** `postmanpat-rulesgen`: same built image, `command:` override (bypasses the entrypoint and its cron exactly like `postmanpat-watch`), the stage-01 report directory mounted read-only at the same in-container path, the SQLite file under a writable host bind (`./rulesgen-data`), port 8092 exposed on the private network only — no published ports, no auth. The image runs as root, so the root-owned `0600` report files written by the cron container are readable in-container; the host-user readability caveat does not apply.
- **Docs.** README gains a rulesgen section; AGENTS.md gains the package, subcommand, and compose service.

## Testing Decisions

Good tests verify external behavior only — what lands in the store from report files, what the page renders, what idempotency means as an observable outcome. No mocks: real temp SQLite databases, real fixture files on disk, `httptest` against the real handler. Fixtures are synthetic reports matching the live report schema exactly (four lens sections, `suppressed` annotations, examples arrays — verified against the working nightly report); no real mailbox data is committed.

- **Ingestion tests** (fixture files in a temp dir → real temp DB): initial ingest lands Clusters with `first_seen == last_seen == generated_at`; re-ingesting the same file changes nothing (no duplicates); a newer report that re-contains a Cluster refreshes count/examples and advances `last_seen` to the newer `generated_at`; a Cluster absent from the newer report remains with its stale data; `template_lens` Clusters never appear; a cluster suppressed for both lanes is ingested but excluded from the pending set; a corrupt file is skipped while others ingest; multi-file order is deterministic.
- **Store tests**: upsert and pending-query semantics at the store seam, where they are cheapest to assert.
- **HTTP tests**: the queue page shows exactly the undecided, not-suppressed-for-both Clusters with lens, keys, count, examples, badges, and `last_seen`; `/healthz` returns 200.
- **CLI tests**: flag defaults and command wiring, following the existing thin cli test style.
- **Not automated**: the compose service definition — no shell harness exists in the repo and this spec introduces none; verified by inspection and the dogfood check.

## Out of Scope

- Decision capture, Snooze, fragment rendering, and merge marking — delivery stages 03–05.
- Authentication, TLS, or any exposure beyond the private network.
- Report retention/history — timestamped accumulation is rejected by ADR 0004; overwrite is the design.
- Any mailbox access or watch/cleanup config editing — rulesgen reads report files only (ADR 0004).
- OTel instrumentation of rulesgen — extending observability is a separate decision.
- Retiring the Python generator or syncing decision state with it — the two records deliberately diverge (ADR 0003).
- Publishing the delivery stubs to GitHub Issues — still deferred; the stubs live in `docs/issues/` and link here.

## Further Notes

- Dogfood acceptance (per the issue-02 handoff): point the service at the real mounted report directory, browse the queue, restart the service, and confirm idempotent re-ingestion and stable queue state.
- The report files (and, likewise, the new store file) are root-owned on the host bind mounts; that is expected — both are read and written by containers, not by the host user.
- The `decisions` table is deliberately unused in this slice. Its presence is the whole point: stage 03 (decision capture) becomes INSERT-only with no schema migration.
