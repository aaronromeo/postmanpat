# 03 — rulesgen: decision capture and fragment rendering

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0002, ADR 0003; CONTEXT.md terms: Cluster Decision, One-Time Cleanup, Ongoing Cleanup, Pending Merge.
> **Spec:** `docs/superpowers/specs/2026-09-14-rulesgen-decision-capture-fragments.md`

**What to build:** Each cluster in the Review Queue gains decision controls, per rule-type lane: **Generate**, **Decline**, **Ignore** (with the script's "also ignore for cleanup?" cross-offer), **Snooze** (hidden until a newer report containing the cluster arrives). Generation forms are script-parity: watch lanes emit client matchers (`list_id_regex` / `sender_regex`+options / `recipient_tag_regex` with escaped defaults from cluster keys); cleanup lanes emit server matchers with the one-rule-per-recipient-alias split, plus the sanctioned extension — `age_window.min`, default empty for One-Time Cleanup and `30d` for Ongoing Cleanup. Every decision is persisted; Generated rules and Ignored entries render as four fragment kinds (watch, cleanup-ongoing, cleanup-onetime, ignore) into the mounted config dir. A Decisions view lists non-pending decisions and allows reversal (undecline → cluster returns to the queue on next ingestion). Rule-building logic is Go-side and must stay at parity with the Python script (ADR 0003).

**Blocked by:** 02 — rulesgen: report ingestion, decision store, read-only Review Queue.

**Status:** complete — merged as PR #36 (squash `8170d65`) and deployed on rocketman. Install-time gap fixed by PR #37 (compose `postmanpat-rulesgen` gained the required `--fragments /fragments` + writable mount `${POSTMANPAT_RULESGEN_FRAGMENTS:-./config}`; without it the service exits with "required flag(s)" and the queue is dark). Deploy verified 2026-09-17: service rebuilt and up on the `pi-services` network, `/healthz` ok, fragment files rendering into the mounted config dir (`/opt/docker/rocketman/postmanpat-config`), SQLite store preserved and restart-survival confirmed. `go test ./...` green; parity corpus against `bin/postmanpat-generate-rules.py` in `rulesgen/testdata/parity/` (ADR 0003).

- [x] All four outcomes persist per (cluster, lane) and survive restarts
- [x] Generated YAML matches the Python script's output shape for equivalent inputs (parity test corpus recommended)
- [x] Multi-alias cleanup recipients split one-rule-per-alias, as the script does
- [x] `age_window.min` offered on both cleanup lanes with the agreed defaults; never offered on watch
- [x] Ignore authoring produces an ignore fragment matching ADR 0002 semantics (`list_ids`, `sender_domains`, `recipient_tags`)
- [x] Suppressed annotations disable the corresponding lane; both-suppressed clusters never enter the queue
- [x] Snoozed clusters reappear when a newer ingested report contains them
- [x] Four fragment files render into the mounted config dir
