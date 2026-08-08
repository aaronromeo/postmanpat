# Suppression travels via analyze report annotation; the generator authors ignore entries

ADR 0001 split ignore filtering across two programs: `analyze` filters Fully Decided messages before aggregation, and per-type (half-decided) suppression happens at the rule generator's prompt. That split stands. What changes is the transport for the per-type half: instead of the generator reading the analyze config (`--config`) and re-implementing the match semantics against lossy cluster keys (capped examples, best-effort subjects), `analyze` computes suppression while aggregating — it already sees every message — and annotates each cluster with `"suppressed": ["watch"]` and/or `["cleanup"]`. The generator reads the annotation and contains no ignore-matching logic. Additionally, the generator now *authors* ignore entries interactively (an `i` answer on a rule prompt) into an `--ignore-out` YAML fragment, following the same generate → review → merge workflow as the watch/cleanup outputs; it never edits the hand-maintained config in place (PyYAML round-trips strip comments).

## Considered Options

- **Script-side matching via `--config`** (ADR 0001's original mechanism): duplicates the match semantics in Python against cluster keys, with best-effort holes for subjects. Rejected.
- **Sidecar suppressions file**: keeps the report byte-pristine but adds a second temp file through the docker `TMPDIR` mount and name-mangling discovery. Rejected as more moving parts for cosmetic purity.
- **Unified single ignore list (no per-type)**: reconsidered and rejected again — the newsletter case (never watch-rule, do cleanup-rule) still wants per-type suppression; see ADR 0001.

## Consequences

- The report schema gains an additive, optional per-cluster field; old reports (no field) and old generators (unknown field) interoperate in both directions.
- `--no-ignore` disables filtering *and* annotation.
- Suppression aggregates as "any message in the cluster matches": exact for key identities (domains, List-IDs, tags), but a mixed-membership cluster can end up annotated for both rule types even when its surviving messages don't individually match both lists — the generator then skips it entirely. Accepted cluster-granularity trade-off.
- The "best-effort" caveats of the script-side design disappear: subjects and addresses now match at message granularity in Go.
- Interactive authoring covers `list_ids`, `sender_domains`, `recipient_tags`; `sender_addresses` and `subject_substrings` remain hand-authored (not derivable from cluster keys).
- The glossary gains *Suppressed* (CONTEXT.md); *Fully Decided* filtering is unchanged.
