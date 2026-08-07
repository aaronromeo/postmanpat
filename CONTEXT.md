# PostmanPat

PostmanPat is a single-user IMAP mailbox triage tool: it analyzes mailbox contents and helps the user generate and apply rules that watch incoming mail or clean up stored mail.

## Language

**Lens**:
One of the fixed grouping perspectives that `analyze` applies to scanned messages (list, sender+unsubscribe, sender+subject-template, recipient-tag). Each lens groups messages that share a Lens Key.
_Avoid_: index, view, grouping

**Lens Key**:
The normalized identity value(s) defining a Cluster within a Lens — e.g. a lowercased sender domain, a List-ID, a recipient tag, a normalized subject.
_Avoid_: key fields, cluster id

**Cluster**:
The set of scanned messages sharing one Lens Key, reported with a count, latest date, signals, and examples.
_Avoid_: group, bucket

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

**Checkpoint**:
The watch command's persisted IMAP UID position, letting it resume after reconnects. Configured via the top-level `checkpoint.path`.
_Avoid_: state file, cursor

**Generation Checkpoint**:
The rule generator script's local JSON record of cluster IDs already presented interactively, so they are not re-prompted. Distinct from the Ignore List: the Generation Checkpoint remembers what was *asked*; the Ignore List records what was *decided*. Fully Decided clusters never appear in reports and never reach it; clusters skipped via a Suppressed annotation are not checkpointed; clusters the user answers — including ones they mark ignored — are checkpointed.
_Avoid_: checkpoint (ambiguous with the watch Checkpoint)
