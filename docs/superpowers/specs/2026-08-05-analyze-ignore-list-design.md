# Analyze Ignore List

## Problem Statement

When I run `analyze` and feed the report into the rule generator, I am re-shown Clusters for senders, lists, and recipient tags I have already made decisions about. The report is cluttered with known mail, and the generator offers me rules — watch rules, cleanup rules, or both — for things I never want automated. There is no way to say "I have decided about this identity; stop asking." The generator's Generation Checkpoint only remembers what I was *asked*; it cannot express what I have *decided*, and it keys on cluster IDs, so a Cluster whose membership changes can resurface.

## Solution

An **Ignore List**, authored once in the analyze config, split into a **Watch Ignore List** and a **Cleanup Ignore List**.

- An identity on **both** lists is **Fully Decided**: `analyze` filters matching messages before aggregation, so they never appear in the report. The report stays completely silent about ignored mail.
- An identity on **one** list stays in the report; the rule generator reads the same config and suppresses only the corresponding rule-type prompt. This keeps the read-it-now, auto-delete-it-later workflow expressible (watch-ignored, cleanup-eligible).
- `analyze --no-ignore` bypasses filtering so I can periodically audit what I am ignoring.

Vocabulary follows the project glossary (`CONTEXT.md`): Lens, Lens Key, Cluster, Ignore List, Watch/Cleanup Ignore List, Fully Decided, Generation Checkpoint. The split-filtering architecture is recorded in `docs/adr/0001-ignore-list-split-filtering.md`.

## User Stories

1. As the mailbox owner, I want to list sender domains I never want watch rules for, so that important senders are never auto-moved or auto-deleted on arrival.
2. As the mailbox owner, I want to list sender domains I never want cleanup rules for, so that stored mail from those senders is never bulk-deleted.
3. As the mailbox owner, I want an identity on both lists to disappear from the analyze report entirely, so that each report shows only undecided mail.
4. As the mailbox owner, I want the report's total scanned count to reflect only un-ignored messages, so that numbers I see describe my undecided mail.
5. As the mailbox owner, I want no trace of ignored mail in the report — no counts, no suppressed-cluster sections — so that the report stays clean.
6. As the mailbox owner, I want a `--no-ignore` flag on analyze, so that I can audit everything I am ignoring without editing config.
7. As the mailbox owner, I want sender-domain entries to match exactly, so that `github.com` does not accidentally ignore `notgithub.com` or `github.com.evil-shop.net`.
8. As the mailbox owner, I accept that exact domain matching does not cover subdomains, so that I consciously list `emails.github.com` when I want it ignored.
9. As the mailbox owner, I want sender-address, List-ID, recipient-tag, and subject entries to match as case-insensitive substrings, so that entries are forgiving to write.
10. As the mailbox owner, I want subject entries to match raw subjects as I see them in my mail client, so that I never have to learn the normalized-subject placeholder format.
11. As the mailbox owner, I want a message ignored when ANY of its sender domains exactly matches a domain entry, so that multi-domain messages are handled sensibly.
12. As the mailbox owner, I want the ignore section to live in the analyze config, so that there is exactly one place to edit.
13. As the mailbox owner, I want both sub-lists optional with empty meaning "nothing ignored", so that the feature is invisible until I opt in.
14. As the mailbox owner, I want the rule generator to read the same analyze config via a `--config` flag, so that the lists exist in exactly one place.
15. As the mailbox owner, I want the generator to run unchanged without `--config`, so that existing workflows and muscle memory keep working.
16. As the mailbox owner, I want the generator to skip the watch prompt but still ask the cleanup prompt for a watch-ignored Cluster, so that the newsletter workflow (keep arriving, auto-delete when old) is one decision, not two.
17. As the mailbox owner, I want the generator to skip the cleanup prompt but still ask the watch prompt for a cleanup-ignored Cluster, symmetrically.
18. As the mailbox owner, I want Clusters suppressed by the Ignore List to never be written to the Generation Checkpoint, so that removing an entry from a list makes its Clusters reappear on the next run.
19. As the mailbox owner, I want ignore matching to go off Lens Key values, so that what I see in the report's keys is exactly what I write in the config.
20. As the mailbox owner, I want invalid ignore config to fail fast at load/validate time, so that a typo never silently disables a suppression I rely on.
21. As the mailbox owner, I want the ignore section accepted (but inert) in watch and cleanup configs, so that shared config parsing never breaks the runtime commands.
22. As the operator, I want per-rule analyze runs to all honor the same global ignore section, so that multi-rule analyze configs behave predictably (one report per rule, each filtered).

## Implementation Decisions

- **Config model (appconfig module).** A new top-level `ignore` section with two optional subsections, `watch` and `cleanup`, each carrying five optional list fields: `sender_domains`, `sender_addresses`, `list_ids`, `recipient_tags`, `subject_substrings`. Because config loading is shared by all commands, the section parses everywhere and is acted on only by analyze and the rule generator. Validation rejects malformed entries at load time; empty or absent lists mean nothing is ignored.
- **Match semantics.** `sender_domains` entries match exactly (after lowercasing) against each of a message's sender domains — any match ignores the message. The other four field types match as case-insensitive substrings. Subject entries match the raw subject, never the normalized one. Entries are matched against the same normalized values that form Lens Keys.
- **Fully Decided filtering (analyze command).** A message is filtered before aggregation when it matches at least one entry on the Watch Ignore List AND at least one entry on the Cleanup Ignore List. Filtering happens after fetch, before report building, at message granularity — this is the only message-exact filtering point in the system.
- **Report silence.** The report schema is unchanged: no ignored-message counts, no suppressed-cluster section, no config echo. `total_messages_scanned` counts only un-ignored messages.
- **Bypass flag.** `analyze --no-ignore` disables ignore filtering for that run. The rule generator has no bypass; auditing happens through the report.
- **Rule generator changes (Python helper).** A new optional `--config` argument pointing at the analyze YAML. Without it, behavior is identical to today. With it, per Cluster: watch-listed match → suppress the watch prompt, still ask cleanup; cleanup-listed match → suppress the cleanup prompt, still ask watch; a Cluster matching both lists (possible when the report was produced with `--no-ignore`) → skip entirely.
- **Generator matching granularity.** The generator sees Clusters, not messages, so suppression matches against Lens Keys: List-ID for the list lens, sender domains for the sender and template lenses, recipient tag for the recipient-tag lens. Sender-address and subject entries are matched best-effort against cluster example data (examples are capped by `--examples`). This is an accepted limitation; the guaranteed path is both-lists filtering in analyze.
- **Checkpoint separation.** Suppressed Clusters are never written to the Generation Checkpoint. The Generation Checkpoint's semantics (interactive decision memory) are otherwise unchanged.
- **Ignore section is global.** Analyze produces one report per rule; the same ignore section applies to every rule's scan.

## Testing Decisions

Good tests here verify external behavior only — config accepted/rejected, messages present or absent in the report, prompts asked or suppressed — never internal data structures. No mocks: the codebase's convention is real in-memory IMAP servers, and that convention holds. Four existing seams are used; no new test infrastructure is introduced.

- **appconfig tests** — load/validate of the `ignore` section using the existing inline-YAML-to-temp-file pattern: both subsections populated, absent section, empty lists, malformed entries.
- **Analyze pure-function seam** — the filtering/report-building path tested with hand-built message fixtures (prior art: the existing report-building tests). This seam carries the semantics table: exact domains, substring others, any-domain matching, Fully Decided (both lists) filtered vs half-decided (one list) retained, raw-subject matching, case-insensitivity.
- **Analyze command seam** — the command executed end-to-end via the root command against the in-memory IMAP server (prior art: the existing analyze command test): ignored messages absent from the written report, silence (no ignored counts in JSON), and `--no-ignore` restoring them.
- **Python seam** — the generator's suppression logic imported via `importlib` (prior art: the existing script test that loads the module without executing `main()`): per-lens prompt suppression, both-listed Cluster skipped, no checkpoint writes for suppressed Clusters, absent `--config` means zero behavior change.

## Out of Scope

- **Runtime enforcement in watch and cleanup.** The Ignore List blocks rule *generation*; it is not a safety rail. Running watch/cleanup commands do not read it and will still apply any rule the user merges into their runtime configs.
- **Regex or glob matching** in ignore entries (substrings and exact domains only).
- **Subdomain matching** for domain entries.
- **Per-entry metadata** (reasons, comments, expiry) — plain string lists only.
- **Changes to the Generation Checkpoint format**, and any migration of existing checkpoint files.
- **Publishing this spec to GitHub Issues** (deferred; gh auth needs fixing and the `ready-for-agent` label does not exist yet).
- **Observability instrumentation** for ignore filtering (no spans/metrics for ignored messages; consistent with report silence).

## Further Notes

- **Cross-identity edge:** Fully Decided is evaluated per message as "matches the watch list AND matches the cleanup list", not "the same identity on both lists". A message watch-ignored via its domain and cleanup-ignored via its List-ID is treated as Fully Decided and filtered. This is conservative and accepted.
- **Deployment reality:** real per-environment configs live outside this repo (the rocketman repo, mounted into the container). Only the example config lives here; the ignore section will be documented for the analyze config but real lists are edited on the host.
- **Naming:** all terminology follows `CONTEXT.md`. The architectural split (analyze filters Fully Decided; the generator suppresses per type) and its rejected alternatives are recorded in `docs/adr/0001-ignore-list-split-filtering.md`.
