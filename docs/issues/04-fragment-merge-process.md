# 04 — Fragment merge process (store semantics + headless surface + runbook)

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0005; CONTEXT.md term: Pending Merge.

**What to build:** The merge process becomes real before it is web-exposed. The store tracks merged/unmerged per decision; each fragment file is re-rendered wholesale from unmerged decisions only, so a fragment always equals exactly the Pending Merge set (ADR 0005 — no append, no timestamped files). A headless surface (`postmanpat rulesgen mark-merged ...`) marks decisions merged from the CLI. A runbook documents the loop: which fragment merges into which live config (watch → `config_watch.yaml`, ongoing → `config_cleanup.yaml`, one-time → the hand-run cleanup-onetime config, ignore → the analyze config's `ignore:` section), how a One-Time Cleanup is executed and retired, and the fact that merged rules take effect on the next cron tick / watch config reload — no restarts.

**Blocked by:** 03 — rulesgen: decision capture and fragment rendering.

**Status:** stub

- [ ] Decisions carry merged/unmerged state; marking merged stops a decision rendering into fragments
- [ ] Fragment re-render is wholesale from unmerged decisions; verified that partial merges leave skipped rules in the fragment
- [ ] CLI marking works without the web UI running
- [ ] Runbook covers all four fragment kinds and the One-Time Cleanup run/retire lifecycle
- [ ] No code path in rulesgen ever writes to the live configs (fragments only)
