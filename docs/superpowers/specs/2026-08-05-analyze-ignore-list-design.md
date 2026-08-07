# Analyze Ignore List

## Problem Statement

When I run `analyze` and feed the report into the rule generator, I am re-shown Clusters for senders, lists, and recipient tags I have already made decisions about. The report is cluttered with known mail, and the generator offers me rules — watch rules, cleanup rules, or both — for things I never want automated. There is no way to say "I have decided about this identity; stop asking." The generator's Generation Checkpoint only remembers what I was *asked*; it cannot express what I have *decided*, and it keys on cluster IDs, so a Cluster whose membership changes can resurface.

## Solution

An **Ignore List**, authored once in the analyze config, split into a **Watch Ignore List** and a **Cleanup Ignore List**.

- An identity on **both** lists is **Fully Decided**: `analyze` filters matching messages before aggregation, so they never appear in the report. The report stays completely silent about ignored mail.
- An identity on **one** list stays in the report, but `analyze` annotates each affected cluster with `"suppressed": ["watch"]` (and/or `["cleanup"]`), computed at message granularity while aggregating. The rule generator reads the annotation and simply never asks the suppressed prompt — it contains no ignore-matching logic and never reads the config.
- The generator also **authors** entries: answer `i` on a rule prompt to record the cluster's identity into an `--ignore-out` YAML fragment, which is reviewed and merged into the analyze config exactly like the existing watch/cleanup outputs.
- `analyze --no-ignore` bypasses filtering and annotation so I can periodically audit what I am ignoring.

Vocabulary follows the project glossary (`CONTEXT.md`): Lens, Lens Key, Cluster, Ignore List, Watch/Cleanup Ignore List, Fully Decided, Suppressed, Generation Checkpoint. The architecture is recorded in `docs/adr/0001-ignore-list-split-filtering.md` (split filtering) as amended by `docs/adr/0002-suppression-via-report-annotation.md` (annotation transport + generator authoring).

## User Stories

1. As the mailbox owner, I want to list sender domains I never want watch rules for, so that important senders are never auto-moved or auto-deleted on arrival.
2. As the mailbox owner, I want to list sender domains I never want cleanup rules for, so that stored mail from those senders is never bulk-deleted.
3. As the mailbox owner, I want an identity on both lists to disappear from the analyze report entirely, so that each report shows only undecided mail.
4. As the mailbox owner, I want the report's total scanned count to reflect only un-ignored messages, so that numbers I see describe my undecided mail.
5. As the mailbox owner, I want no trace of Fully Decided mail in the report — no counts, no suppressed-cluster sections — so that the report stays clean.
6. As the mailbox owner, I want a `--no-ignore` flag on analyze, so that I can audit everything I am ignoring without editing config.
7. As the mailbox owner, I want sender-domain entries to match exactly, so that `github.com` does not accidentally ignore `notgithub.com` or `github.com.evil-shop.net`.
8. As the mailbox owner, I accept that exact domain matching does not cover subdomains, so that I consciously list `emails.github.com` when I want it ignored.
9. As the mailbox owner, I want sender-address, List-ID, recipient-tag, and subject entries to match as case-insensitive substrings, so that entries are forgiving to write.
10. As the mailbox owner, I want subject entries to match raw subjects as I see them in my mail client, so that I never have to learn the normalized-subject placeholder format.
11. As the mailbox owner, I want a message ignored when ANY of its sender domains exactly matches a domain entry, so that multi-domain messages are handled sensibly.
12. As the mailbox owner, I want the ignore section to live in the analyze config, so that there is exactly one place to edit.
13. As the mailbox owner, I want both sub-lists optional with empty meaning "nothing ignored", so that the feature is invisible until I opt in.
14. As the mailbox owner, I want the generator to learn suppressions from the report itself, so there is no extra flag to pass and no second implementation of the matching rules to drift out of sync.
15. As the mailbox owner, I want the generator to skip the watch prompt but still ask the cleanup prompt for a watch-ignored Cluster, so that the newsletter workflow (keep arriving, auto-delete when old) is one decision, not two.
16. As the mailbox owner, I want the generator to skip the cleanup prompt but still ask the watch prompt for a cleanup-ignored Cluster, symmetrically.
17. As the mailbox owner, I want a Cluster annotated for both rule types to be skipped entirely without touching the Generation Checkpoint, so that removing the config entry makes it reappear.
18. As the mailbox owner, I want half-suppressed Clusters that I am actually prompted about to be checkpointed like any other answered cluster, so I am never asked the same question twice.
19. As the mailbox owner, I want ignore matching to happen against full message data inside analyze, so that subject and address entries match exactly — not approximately against capped report examples.
20. As the mailbox owner, I want invalid ignore config to fail fast at load/validate time, so that a typo never silently disables a suppression I rely on.
21. As the mailbox owner, I want the ignore section accepted (but inert) in watch and cleanup configs, so that shared config parsing never breaks the runtime commands.
22. As the operator, I want per-rule analyze runs to all honor the same global ignore section, so that multi-rule analyze configs behave predictably (one report per rule, each filtered).
23. As the mailbox owner, I want to press `i` while reviewing a Cluster to add its identity to an ignore output file, so that recording a decision is one keystroke.
24. As the mailbox owner, I want to choose at authoring time whether the entry applies to watch, cleanup, or both, so per-type decisions stay expressible.
25. As the mailbox owner, I want authored entries written to a separate mergeable YAML fragment, so my hand-maintained config (and its comments) is never rewritten by a tool.
26. As the mailbox owner, I want the fragment deduplicated and sorted, so merging it into the real config is trivial.
27. As the mailbox owner, I want clusters I just marked ignored to be checkpointed, so I am not re-asked about them in the gap before I merge the fragment and rerun analyze.

## Implementation Decisions

- **Config model (appconfig module).** A new top-level `ignore` section with two optional subsections, `watch` and `cleanup`, each carrying five optional list fields: `sender_domains`, `sender_addresses`, `list_ids`, `recipient_tags`, `subject_substrings`. Because config loading is shared by all commands, the section parses everywhere and is acted on only by analyze. Validation rejects malformed entries at load time; empty or absent lists mean nothing is ignored.
- **Match semantics.** `sender_domains` entries match exactly (after lowercasing) against each of a message's sender domains — any match ignores the message. The other four field types match as case-insensitive substrings. Subject entries match the raw subject, never the normalized one. Entries are matched against the same normalized values that form Lens Keys.
- **Fully Decided filtering (analyze command).** A message is filtered before aggregation when it matches at least one entry on the Watch Ignore List AND at least one entry on the Cleanup Ignore List. Filtering happens after fetch, before report building, at message granularity.
- **Report silence for Fully Decided mail.** `total_messages_scanned` counts only un-ignored messages; no ignored-message counts or sections exist. Half-decided clusters are not ignored mail — they stay visible and carry the annotation below.
- **Suppressed annotation.** Each cluster may carry `"suppressed": ["watch"]` and/or `["cleanup"]`, computed during aggregation: a cluster is suppressed for a rule type when ANY of its messages matches that type's ignore list (OR-aggregation, reusing the same message-level matcher). The field is omitted when empty; the schema change is additive and optional, so old reports and old generators interoperate in both directions.
- **Bypass flag.** `analyze --no-ignore` disables both filtering and annotation for that run. The rule generator has no bypass; auditing happens through the report.
- **Generator suppression.** The generator reads `suppressed` from each cluster: one side listed → that prompt is skipped with a printed note, the other is asked; both sides listed → the cluster is skipped entirely (no prompt, no Generation Checkpoint write) and included in a one-line summary count. The generator has no config flag and no matching code.
- **Generator authoring.** Rule prompts accept `i` (ignore). On the watch prompt, `i` records the identity for watch and asks "Also ignore for cleanup?" — yes records cleanup too and skips the cleanup prompt. On the cleanup prompt, `i` records cleanup only. Identity mapping: `list_lens` → `list_ids` (the ListID), `sender_unsub_lens` → `sender_domains` (all domains of the cluster), `recipient_tag_lens` → `recipient_tags`. `sender_addresses` and `subject_substrings` are not offered interactively (not derivable from cluster keys) and remain hand-authored.
- **Authoring output.** A new optional `--ignore-out` path receives a YAML fragment of shape `ignore: {watch: {...}, cleanup: {...}}` — deduplicated, sorted, empty sides omitted — written with the generator's existing YAML serializer. If entries were authored without `--ignore-out`, a warning says so. The analyze config is never edited in place (PyYAML round-trips strip comments from a hand-maintained file).
- **Generation Checkpoint.** Records every interactively answered cluster, including ones marked ignored with `i` (so they are not re-asked before the fragment is merged and analyze rerun). Clusters skipped via the annotation are not checkpointed — config-driven suppression is deterministic and needs no memory.
- **Ignore section is global.** Analyze produces one report per rule; the same ignore section applies to every rule's scan.

## Testing Decisions

Good tests here verify external behavior only — config accepted/rejected, messages present or absent in the report, annotations present or absent, prompts asked or suppressed, fragment contents — never internal data structures. No mocks in Go (the codebase's convention is real in-memory IMAP servers); Python prompt flows use `unittest.mock` against the prompt function. Four existing seams are used; no new test infrastructure is introduced.

- **appconfig tests** — load/validate of the `ignore` section using the existing inline-YAML-to-temp-file pattern: both subsections populated, absent section, empty lists, malformed entries.
- **Analyze pure-function seam** — hand-built message fixtures (prior art: the existing report-building tests). Carries the semantics table (exact domains, substring others, any-domain matching, Fully Decided filtered vs half-decided retained, raw-subject matching, case-insensitivity) AND the annotation: with an ignore config set, surviving clusters carry the expected `suppressed` values; with none, the field is absent.
- **Analyze command seam** — the command executed end-to-end via the root command against the in-memory IMAP server: Fully Decided messages absent from the written report, half-decided clusters annotated, `--no-ignore` restoring messages and removing annotations, and silence (no ignored counts in the JSON).
- **Python seam** — the generator imported via `importlib` (prior art: the existing script test): prompt-flow tests with hand-built cluster dicts carrying `suppressed` (mocked prompt function, captured stdout) verify skip/note/checkpoint behavior; authoring tests cover the identity-extraction, merge, dedup, and fragment-building functions plus the `i`-answer flow (including the "also cleanup" follow-up skipping the second prompt).

## Out of Scope

- **Runtime enforcement in watch and cleanup.** The Ignore List blocks rule *generation*; it is not a safety rail. Running watch/cleanup commands do not read it and will still apply any rule the user merges into their runtime configs.
- **Regex or glob matching** in ignore entries (substrings and exact domains only).
- **Subdomain matching** for domain entries.
- **Per-entry metadata** (reasons, comments, expiry) — plain string lists only.
- **Interactive authoring of `sender_addresses` and `subject_substrings`** — hand-authored config fields only.
- **In-place editing of the analyze config** by the generator (comment loss); the `--ignore-out` fragment workflow is the authoring path.
- **Changes to the Generation Checkpoint format**, and any migration of existing checkpoint files.
- **Publishing this spec to GitHub Issues** (deferred; gh auth needs fixing and the `ready-for-agent` label does not exist yet).
- **Observability instrumentation** for ignore filtering (no spans/metrics for ignored messages; consistent with report silence).

## Further Notes

- **Cross-identity edge:** Fully Decided is evaluated per message as "matches the watch list AND matches the cleanup list", not "the same identity on both lists". A message watch-ignored via its domain and cleanup-ignored via its List-ID is treated as Fully Decided and filtered. Conservative, accepted.
- **OR-aggregation edge:** a mixed-membership cluster (some messages match watch, others match cleanup) can be annotated for both rule types even though its surviving messages don't individually match both lists — the generator then skips it entirely. Accepted cluster-granularity trade-off, documented in ADR 0002.
- **Deployment reality:** real per-environment configs live outside this repo (the rocketman repo, mounted into the container). The `--ignore-out` fragment is merged there by hand, like the watch/cleanup outputs today.
- **Naming:** all terminology follows `CONTEXT.md`. Architecture: `docs/adr/0001-ignore-list-split-filtering.md` (split filtering), amended by `docs/adr/0002-suppression-via-report-annotation.md` (annotation transport + generator authoring).
