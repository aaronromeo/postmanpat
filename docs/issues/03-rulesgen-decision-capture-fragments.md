# 03 — rulesgen: decision capture and fragment rendering

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0002, ADR 0003; CONTEXT.md terms: Cluster Decision, One-Time Cleanup, Ongoing Cleanup, Pending Merge.

**What to build:** Each cluster in the Review Queue gains decision controls, per rule-type lane: **Generate**, **Decline**, **Ignore** (with the script's "also ignore for cleanup?" cross-offer), **Snooze** (hidden until a newer report containing the cluster arrives). Generation forms are script-parity: watch lanes emit client matchers (`list_id_regex` / `sender_regex`+options / `recipient_tag_regex` with escaped defaults from cluster keys); cleanup lanes emit server matchers with the one-rule-per-recipient-alias split, plus the sanctioned extension — `age_window.min`, default empty for One-Time Cleanup and `30d` for Ongoing Cleanup. Every decision is persisted; Generated rules and Ignored entries render as four fragment kinds (watch, cleanup-ongoing, cleanup-onetime, ignore) into the mounted config dir. A Decisions view lists non-pending decisions and allows reversal (undecline → cluster returns to the queue on next ingestion). Rule-building logic is Go-side and must stay at parity with the Python script (ADR 0003).

**Blocked by:** 02 — rulesgen: report ingestion, decision store, read-only Review Queue.

**Status:** stub

- [ ] All four outcomes persist per (cluster, lane) and survive restarts
- [ ] Generated YAML matches the Python script's output shape for equivalent inputs (parity test corpus recommended)
- [ ] Multi-alias cleanup recipients split one-rule-per-alias, as the script does
- [ ] `age_window.min` offered on both cleanup lanes with the agreed defaults; never offered on watch
- [ ] Ignore authoring produces an ignore fragment matching ADR 0002 semantics (`list_ids`, `sender_domains`, `recipient_tags`)
- [ ] Suppressed annotations disable the corresponding lane; both-suppressed clusters never enter the queue
- [ ] Snoozed clusters reappear when a newer ingested report contains them
- [ ] Four fragment files render into the mounted config dir
