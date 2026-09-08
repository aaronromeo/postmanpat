# 02 — rulesgen: report ingestion, decision store, read-only Review Queue

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0003, ADR 0004; CONTEXT.md terms: rulesgen, Review Queue, Cluster Decision, Analyze Report.
> **Spec:** `docs/superpowers/specs/2026-09-03-rulesgen-readonly-queue-design.md`

**What to build:** `postmanpat rulesgen serve` runs as a new docker-compose service (private network only, no auth in v1). It ingests Analyze Report files from the mounted report directory into a SQLite store and renders a read-only Review Queue: every Pending cluster with its lens, keys, count, examples, Suppressed badges, and `last_seen`. No decision controls yet — this slice proves the pipeline and is dogfoodable against manually produced reports. Clusters dedupe by cluster ID across reports; Pending clusters persist across reports (the 36h sliding window means a cluster appears only when mail recently arrived) and refresh counts/examples when re-ingested; `template_lens` clusters are not ingested (script parity — it never presents them). The store schema anticipates three rule-type lanes (watch / one-time cleanup / ongoing cleanup) even though no decisions are captured yet.

**Blocked by:** None — can start immediately, in parallel with 01 (develop against reports from the existing one-off analyze workflow).

**Status:** code complete (spec: `docs/superpowers/specs/2026-09-03-rulesgen-readonly-queue-design.md`). Package `rulesgen/` + `cli/rulesgen.go` + `postmanpat-rulesgen` compose service; `go test ./...` green; dogfooded against the live nightly report (61 pending clusters rendered; restart with same store shows identical, idempotent queue). Pending: live deploy on rocketman, then the first box.

- [ ] Compose service starts on the private network and serves the queue page (pending live deploy)
- [x] Ingestion is idempotent: re-ingesting the same report file creates no duplicates
- [x] Queue shows exactly the undecided, unsuppressed-for-both clusters from ingested reports, with `last_seen`
- [x] A cluster absent from the latest report but never decided remains Pending with its stale data
- [x] A cluster re-appearing in a newer report refreshes count/examples/`last_seen`
- [x] `template_lens` clusters never appear
- [x] Go tests for ingestion + store; `go test ./...` stays green
