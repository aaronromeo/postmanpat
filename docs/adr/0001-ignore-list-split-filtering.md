# Ignore List filtering is split between analyze and the rule generator

The Ignore List (see CONTEXT.md) blocks rule generation for already-decided identities, and is split into a Watch Ignore List and a Cleanup Ignore List. Filtering is deliberately split across two programs: `analyze` filters Fully Decided messages (on both lists) before aggregation, while `generate-rules.py` suppresses only the corresponding rule-type prompt for half-decided identities (on one list). A single-stage design is impossible: the report must stay silent about ignored mail (which requires removal before aggregation), yet half-decided mail must remain in the report so the other rule type can still be generated from it.

## Considered Options

- **Nothing pre-filtered; the script does all suppression.** Rejected: the report would keep listing fully-decided clusters forever, and the user explicitly wants the report to describe only undecided mail.
- **One unified list, everything pre-filtered.** Rejected: it makes the read-it-then-auto-delete-it-later workflow inexpressible (e.g. a newsletter that must never get a watch rule but should get a cleanup rule).
- **Ignore lists echoed into the report JSON for the script to read.** Rejected: the report stays free of config; the script reads the same analyze YAML via its own `--config` flag.

## Consequences

- The ignore lists live in the analyze config; `generate-rules.py` gains a `--config` flag pointing at that same file.
- Message-exact filtering exists only in analyze. The script's per-type suppression is best-effort at cluster granularity (e.g. subject patterns are matched against example subjects, capped by `--examples`).
- `analyze --no-ignore` bypasses filtering for audits; the script has no bypass — auditing happens through the report.
- Clusters suppressed via the ignore lists are never written to the Generation Checkpoint, so removing an entry from a list makes its clusters reappear.
