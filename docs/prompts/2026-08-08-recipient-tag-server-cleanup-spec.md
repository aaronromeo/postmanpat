# Prompt: Design Spec for Recipient-Tag Server-Side Cleanup Rules

Write a design spec for generating **server-side cleanup rules from `recipient_tag_lens` analyze Clusters by emitting rules that use the existing `recipients` server matcher**, replacing the rule generator's current refusal. The spec should follow the format and tone of `docs/superpowers/specs/2026-08-05-analyze-ignore-list-design.md` (Problem Statement, Solution, User Stories, Implementation Decisions, Testing Decisions, Out of Scope, Further Notes) and use the project's established vocabulary from `CONTEXT.md`.

## Context (verified against code)

1. **`recipient_tag_lens` keys are lossy.** `postmanpat analyze` clusters messages under lenses; for `recipient_tag_lens`, each envelope `To` address is transformed by `recipientTag` (`imap/internal/selectors/manager.go:343-345`, pattern `[^a-z0-9]` → `_` at `manager.go:35`) applied per-`To`-address at `manager.go:204`. The transform lowercases then replaces every non-`[a-z0-9]` character with `_`, so `alice.smith+news@example.com` becomes `alice_smith_news_example_com`. It is **lossy and non-invertible**: `.`, `+`, `-`, and `@` all collapse to `_`. Only `To` is transformed, not `Cc`.
2. **Cluster keys are set-based and comma-joined.** `buildRecipientTagLens` (`cli/analyze.go:431-451`) calls `normalizeRecipientTags` (`cli/analyze.go:477-495`, which lowercases, dedupes, and sorts) on the already-transformed tags, then comma-joins them into a single Lens Key `recipient_tag=tag1,tag2` (`cli/analyze.go:438-439`).
3. **The generator currently refuses these Clusters for cleanup.** `bin/postmanpat-generate-rules.py:447-449` prints `"recipient_tag_lens does not support server-side cleanup rules."` and emits no rule. Watch rules are still offered, using the client-side `recipient_tag_regex` matcher (`appconfig/config.go:60`, evaluated post-fetch at `watchrunner/internal/matchers/matcher.go:105-106` against `data.RecipientTags`).
4. **`cleanup` rejects client matchers; `ServerMatchers` has no recipient-tag field.** `cli/cleanup.go` rejects rules carrying client matchers. `ServerMatchers` (`appconfig/config.go:93-105`) exposes `Recipients []string` but no recipient-tag-specific field.
5. **The `recipients` server matcher already does recipient substring search.** It emits `OR (HEADER "To" <value>) (HEADER "Cc" <value>)` criteria (`imap/internal/searches/manager.go:132-146`), as a case-insensitive substring match against both headers. Arbitrary-header-search plumbing already exists (`list_id_substring`, `returnpath_substring` per `README.md:74`). So server-side recipient matching is possible **today** via hand-authored rules like `recipients: ["+news@example.com"]` — the generator simply cannot derive the raw substring from the lossy normalized Lens Key.
6. **The Ignore List matches normalized tags.** `IgnoreMatchers.RecipientTags` (`appconfig/config.go:183`) matches against the same normalized tag values that form the Lens Key, so any change to tag computation or the Lens Key shape has Ignore List consequences (see `CONTEXT.md`: Ignore List, Fully Decided, Suppressed).

## Questions the spec must resolve

- **Obtaining a raw, server-searchable substring from a lossy Lens Key.** The spec must weigh and pick one: (a) change the lens/report to also record raw recipient addresses (e.g. in Cluster `examples` or a new key field) so the generator can derive a `+tag@domain` substring; (b) keep the lens as-is and prompt the user to confirm/edit the derived substring interactively; (c) change the normalization itself. Justify the choice, including the cost/benefit of report-format changes vs. generator-only changes.
- **Semantics gap between the Lens Cluster and the generated `recipients` rule.** `recipients` substring search matches display names too (false positives like `"Evening News" <info@example.com>`), searches **both To and Cc** while the lens is **To-only**, and is case-insensitive substring. The spec must state how the generated rule's semantics differ from the Lens Cluster and whether that divergence is acceptable (or how it is mitigated).
- **Set-based multi-tag keys.** For Clusters whose key is `tag1,tag2` (multiple recipients), does the generator offer one rule per tag, skip, or prompt? State the behavior and rationale.
- **Go vs. Python split.** What (if anything) changes in Go (`analyze` report fields, validation) versus only the Python generator? Be explicit about whether a report-schema change is needed and whether it is additive/optional (per ADR 0002's backward-compatibility precedent).
- **Ignore List interaction.** Does `recipient_tags` Ignore List matching need to change if the report keys or recorded values change? Confirm the existing normalized-tag matching still holds, or describe the required update.
- **Tests.** Generator tests live in `bin/test_generate_rules.py`; identify what new cases are needed. Specify any Go tests the spec requires (e.g. report-field tests using the existing in-memory IMAP server seam, per the project's no-mock convention).

## Constraints

- IMAP logic must remain vendor-neutral (repo rule; no provider-specific headers or assumptions).
- Server-side matchers must remain plain IMAP SEARCH (substring/header/flags/dates) — **no regex server-side**.
- Watch-side behavior (`recipient_tag_regex`) must remain unchanged unless the spec explicitly justifies otherwise.
- Minimal intrusion; follow existing code and doc style. Any report-schema change should be additive and optional so old reports and old generators interoperate (precedent: ADR 0002's `suppressed` annotation).
- Vocabulary follows `CONTEXT.md` (Lens, Lens Key, Cluster, Ignore List, Fully Decided, Suppressed, Generation Checkpoint).

## References

- `README.md:51-75` — server/client matcher lists.
- `CONTEXT.md` — Lens, Lens Key, Cluster, Ignore List, Fully Decided, Suppressed, Generation Checkpoint vocabulary.
- `docs/adr/0001-ignore-list-split-filtering.md` — split filtering architecture.
- `docs/adr/0002-suppression-via-report-annotation.md` — annotation transport, generator authoring, backward-compatible additive report fields.
- `docs/superpowers/specs/2026-08-05-analyze-ignore-list-design.md` — spec format and tone to match.
