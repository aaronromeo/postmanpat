# 05 — Pending Merge in the web UI

> Stub issue — publish to GitHub when the outage ends. Context: ADR 0005; CONTEXT.md term: Pending Merge.

**What to build:** A Pending Merge view in rulesgen exposes the merge process: every unmerged Generated rule and Ignored entry listed per fragment kind, each with a YAML preview of exactly what the fragment carries. Per-rule checkboxes with "mark merged" (plus select-all per kind) handle the reality that hand-merges are selective — skipped rules simply stay in the fragment. Marking merged re-renders the fragment files so they always equal the pending set.

**Blocked by:** 04 — Fragment merge process (store semantics + headless surface + runbook).

**Status:** stub

- [ ] Pending Merge view lists all unmerged decisions grouped by fragment kind, with per-rule YAML preview
- [ ] Per-rule checkboxes + mark-merged; select-all per kind
- [ ] After marking, fragment files re-render without the merged decisions (verified on disk)
- [ ] A decision marked merged by mistake can be un-marked from the Decisions view
