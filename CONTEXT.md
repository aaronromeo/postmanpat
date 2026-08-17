# PostmanPat

PostmanPat is a single-user IMAP mailbox triage tool: it analyzes mailbox contents and helps the user generate and apply rules that watch incoming mail or clean up stored mail.

## Language

### Analysis

**Lens**:
One of the fixed grouping perspectives that `analyze` applies to scanned messages (list, sender+unsubscribe, sender+subject-template, recipient-tag). Each lens groups messages that share a Lens Key.
_Avoid_: index, view, grouping

**Lens Key**:
The normalized identity value(s) defining a Cluster within a Lens — e.g. a lowercased sender domain, a List-ID, a recipient tag, a normalized subject.
_Avoid_: key fields, cluster id

**Cluster**:
The set of scanned messages sharing one Lens Key, reported with a count, latest date, signals, and examples.
_Avoid_: group, bucket

**Analyze Report**:
The JSON artifact produced by one `analyze` run — one report per rule in the analyze config — carrying per-Lens Clusters with counts, signals, examples, and Suppressed annotations. Rule generators consume reports and never see the mailbox. Scheduled runs write to a fixed path, overwriting the previous report (see ADR 0004).
_Avoid_: analysis results, scan output

### Ignore and Suppression

**Ignore List**:
A config-driven set of identities (sender domains matched exactly; sender addresses, List-IDs, recipient tags, and raw subjects matched as case-insensitive substrings) marking mail as already-decided so that no rules are generated for it. It is split into a Watch Ignore List and a Cleanup Ignore List; matching happens against Lens Key values, and the report stays silent about ignored mail.
_Avoid_: exclude list, blocklist, denylist, skip list

**Watch Ignore List**:
The subset of the Ignore List that blocks generation of WATCH rules for matching identities. Matching clusters may still generate cleanup rules.
_Avoid_: watch exclude, watch skip

**Cleanup Ignore List**:
The subset of the Ignore List that blocks generation of CLEANUP rules for matching identities. Matching clusters may still generate watch rules.
_Avoid_: cleanup exclude, cleanup skip

**Fully Decided**:
An identity appearing on both the Watch Ignore List and the Cleanup Ignore List. `analyze` filters fully-decided messages before aggregation, so they never appear in the report. Half-decided identities (one list only) stay in the report; `analyze` annotates their clusters as Suppressed and the generator does not offer that rule type.

**Suppressed**:
A per-cluster report annotation (`suppressed`) naming the rule types — watch and/or cleanup — the rule generator must not offer for that cluster. Computed by `analyze` during aggregation at message granularity; a cluster suppressed for both rule types is skipped without prompting.
_Avoid_: blocked, filtered (filtering is what happens to Fully Decided messages)

### Rule Generation

**rulesgen**:
The web service hosting the Review Queue: it ingests scheduled Analyze Reports, records Cluster Decisions, and renders Pending Merge fragments for hand-merge into the live configs. Coexists with the Python rule-generator script, which remains the offline path with its own Generation Checkpoint.
_Avoid_: the web UI, the generator UI

**Review Queue**:
The working set of Clusters from ingested Analyze Reports that await a Cluster Decision — not Generated, Declined, or Ignored, and not currently Snoozed. Deduplicated by cluster ID across reports. Because scheduled analyze scans a sliding age window, a Cluster appears in a report only when mail recently arrived; Pending Clusters therefore persist across reports, tracking `last_seen`, and refresh their counts/examples when a newer report re-contains them.
_Avoid_: inbox, todo list

**Cluster Decision**:
The persisted record of the outcome for one (Cluster, rule-type lane) pair: Generated, Declined, Ignored, or Snoozed (hidden until a newer report containing the Cluster arrives). The system of record for what was decided in rulesgen; deliberately distinct from the script's Generation Checkpoint, which records what was *asked* (see ADR 0003).
_Avoid_: verdict, resolution

**Pending Merge**:
The set of Generated rules and Ignored entries not yet marked merged, rendered by rulesgen as fragment files (watch, ongoing cleanup, one-time cleanup, ignore) awaiting hand-merge into the live configs. Marking merged is per rule, because hand-merges are selective.
_Avoid_: staged, unapplied

**One-Time Cleanup**:
A cleanup rule that purges an existing backlog whose future mail is covered by a watch rule: merged into a separate one-time cleanup config, run by hand, and retired by hand once the backlog clears. Generated with no `age_window` by default.
_Avoid_: backlog purge, retroactive cleanup

**Ongoing Cleanup**:
A cleanup rule merged into the standing cleanup config that runs on the cleanup schedule, typically with `age_window.min` (default 30d at generation time) to keep the `@` folders trimmed.
_Avoid_: recurring cleanup

### State

**Checkpoint**:
The watch command's persisted IMAP UID position, letting it resume after reconnects. Configured via the top-level `checkpoint.path`.
_Avoid_: state file, cursor

**Generation Checkpoint**:
The rule generator script's local JSON record of cluster IDs already presented interactively, so they are not re-prompted. Distinct from the Ignore List: the Generation Checkpoint remembers what was *asked*; the Ignore List records what was *decided*. Fully Decided clusters never appear in reports and never reach it; clusters skipped via a Suppressed annotation are not checkpointed; clusters the user answers — including ones they mark ignored — are checkpointed. The rulesgen service instead records Cluster Decisions; the two deliberately diverge (see ADR 0003).
_Avoid_: checkpoint (ambiguous with the watch Checkpoint)
