# rulesgen keeps fragment output and manual merge; it never edits live configs

rulesgen renders Generated rules and Ignored entries as fragment files — watch, ongoing cleanup, one-time cleanup, ignore — which the user hand-merges into the live configs, preserving the generate → review → merge workflow established for the script (ADR 0002). Each fragment is re-rendered wholesale from the store's unmerged decisions whenever one changes, so a fragment always equals exactly the Pending Merge set; after hand-merging, the user marks decisions merged per rule (checkboxes), because hand-merges are selective.

## Considered Options

- **The service edits the live configs directly**: rejected — the configs are hand-maintained with comments and deliberate rule ordering (rules are ordered, apply-all), and a generated in-place edit respects neither. This was the PyYAML rationale in ADR 0002; the Go rewrite does not change the principle.
- **Config-as-artifact** (rules become store rows; watch/cleanup/analyze configs are fully rendered and no longer hand-edited): rejected for now — too large a workflow change, and hand placement of rules during merge remains desired.
- **Append-only or timestamped fragment files**: rejected — rules accumulate between irregular merges; appending risks duplicates on re-merge, timestamped files accumulate clutter. Wholesale re-render of the unmerged set keeps one file per kind and loses nothing.

## Consequences

- The store tracks merged/unmerged per decision; "mark merged" only stops a decision from rendering — it never edits a config.
- The ignore fragment keeps ADR 0002's semantics: identities land in the analyze config's `ignore:` section by hand, so Ignore List authorship stays reviewable.
- A rule skipped during a partial merge simply stays in the fragment; nothing is silently dropped.
