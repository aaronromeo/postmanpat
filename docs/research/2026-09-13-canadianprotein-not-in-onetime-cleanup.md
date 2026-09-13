# Why `canadianprotein` Wasn't Picked Up by the One-Time Cleanup

**Date:** 2026-09-13
**Question:** Why did the onetime cleanup (and the analysis it was built from) not pick up `canadianprotein` mail?

## TL;DR

No cleanup rule was ever generated for canadianprotein because **no canadianprotein message arrived during the analyze window**. All 23 canadianprotein messages still sitting in INBOX are from `support@canadianprotein.com`, the newest dated **2026-09-10** — before every analysis window (36h). The `analyze` step is the sole source of cleanup rules and scans INBOX only, so an identity absent from the window produces no rule. Separately, the two *watch* rules don't file this backlog either: the old one matches only the Klaviyo marketing subdomain (`send.`), and watch is incremental (IDLE + UID checkpoint) so it never retroactively applies a rule to mail that predates the checkpoint.

## Findings

### 1. The onetime cleanup config has no canadianprotein rule

`/home/aaron/workspace/rocketman/postmanpat-config/cleanup-onetime.yml` (20 rules) contains no rule matching any canadianprotein identity. The rules were generated from an analyze report (provenance: `/opt/docker/rocketman/postmanpat-config/analyze-out/cleanup-new.yml` contains the identical Willow/Branch/Indigo/Saguaro/Cuzn/Canadian-Labour rules found in the onetime config). With no rule, `cleanup` matches nothing — `rules_matched=0`.

### 2. canadianprotein was not in any analyze report

- Report `/opt/docker/postmanpat/analyze-out/postmanpat-analyze-analyze-inbox.json` (`generated_at 2026-09-13T03:30:06Z`): window `after 2026-09-11T15:30:06Z`, `before 2026-09-13T03:30:06Z` (i.e. `age_window.max: 36h` from `analysis.yml`); `total_messages_scanned: 29`. `grep canadian` → **no matches**. (Also no `protein` matches.)
- The earlier report that generated the onetime config (`postmanpat-analyze-2146502289.json`, 21:28Z Sep 12) is mode `600`/root and unreadable without sudo, but its downstream artifact — the onetime config with its 20 rules — contains no canadianprotein rule either.

### 3. The mail physically sat outside every window

IMAP probe of INBOX (read-only `EXAMINE` + `UID FETCH`):

| Sample UID | Date header | From |
|---|---|---|
| 332046 | 2025-01-24 | Canadian Protein <support@canadianprotein.com> |
| 339533 | 2025-09-09 | support@canadianprotein.com |
| 347717 | 2026-03-06 | support@canadianprotein.com |
| 354611 | 2026-09-07 | support@canadianprotein.com |
| 354729 | 2026-09-10 | support@canadianprotein.com |
| 354746 | 2026-09-10 (newest) | support@canadianprotein.com |

23 messages total, addressed To: `canadianprotein@aaronromeo.com`, **no `List-Unsubscribe` and no `List-ID` header** (92-byte HEADER.FIELDS responses) → transactional/account mail, not marketing. The newest is 2026-09-10, before the 36h analysis windows ending 21:28Z (09-12) and 03:30Z (09-13). A 7d window would have caught them (see §5), but the analyses used 36h. `age_window.max` → `criteria.Since = now-<dur>` (INTERNALDATE) in `imap/internal/searches/manager.go:91-97`.

### 4. The watch rules can't file the backlog

Two watch rules exist in `watch.yml`:

- **"Canadaian Protein"** (`watch.yml:318-324`): `sender_regex: send\.canadianprotein\.com` → `@Promotions`. Client matchers OR their regexes, but there's only one. Matches **only the Klaviyo subdomain** (`klaviyo@send.canadianprotein.com` — confirmed: the 20 messages already in `@Promotions` are all `klaviyo@send.canadianprotein.com` with `List-Unsubscribe`). Does **not** match `support@canadianprotein.com`.
- **"Canadian Protein"** (`watch.yml:1083-1090`): `sender_regex: canadianprotein\.com` + `list_unsubscribe: false` → `@Broadcast`. Added 2026-09-12 ~22:14 (same generator batch as the onetime rules). Its matchers *would* fit the 23 INBOX messages (sender contains `canadianprotein.com`; no `List-Unsubscribe`), but it has never seen them because:

  **Watch is incremental only.** `cli/watch.go:218` calls `client.SearchUIDsNewerThan(cycleCtx, state.LastUID)` — it processes only messages with UID greater than the persisted checkpoint (`cli/watch.go:158-160`, `imap/internal/searches/manager.go:276-298`). Old INBOX mail predating the rule and the checkpoint is never re-scanned. `@Broadcast` currently contains **0** canadianprotein messages (probe), confirming the rule has never been applied.

### 5. Why the analysis missed it is a window-size miss, not an ignore/list issue

canadianprotein is **not** on either ignore list in `analysis.yml` (sender_domains / recipient_tags), so it wasn't filtered as Fully Decided, and it isn't suppressed or min-count-evicted across repeated windows — it's simply outside the 36h windows. A 7d manual analysis (`analysis-manual.yml`, `age_window.max: 7d`) *would* have included the 2026-09-07/10 mail and generated the cluster (4 messages ≥ default `min-count 2`).

## Root Cause

| Rank | Cause | Evidence | Confidence |
|---|---|---|---|
| 1 | No canadianprotein message was in the analyze window → no cleanup rule generated | newest canadianprotein in account is 2026-09-10; both analyses used 36h windows; report grep empty | **Definitive** |
| 2 | Watch is new-mail-only, so the pre-existing 23-message backlog was never retroactively filed | `watch.go:218` `SearchUIDsNewerThan(LastUID)`; @Broadcast = 0 | **Definitive** |
| 3 | Old watch rule targets only the `send.` marketing subdomain | @Promotions = 20 all from `klaviyo@send.canadianprotein.com`; `support@` not matched | **Definitive** |
| 4 | Ignore list / suppression excluded it | not present in `analysis.yml` ignore lists | Ruled out |

## Suggested Fix (options; no changes made — Plan mode)

1. **Add a watch rule** for the transactional variant: `sender_regex: support\.canadianprotein\.com` (or broad `canadianprotein\.com` without a `list_unsubscribe` constraint) → e.g. `@Receipts`. This handles *future* mail; it will **not** file the existing 23 (watch is incremental).
2. **For the existing backlog**: add one cleanup rule (server matcher: `sender_substring: support.canadianprotein.com`, INBOX, no age window) — or re-run analysis with `age_window.max: 7d` and let the generator produce the rule, then run cleanup. Backlog would file on the next cleanup tick.
3. Saguaro-receipts-style separation: marketing (`send.`) stays `@Promotions`/`@Broadcast`; transactional (`support@`) → a receipts destination.

## Method / Sources

- Read-only IMAP probes (EXAMINE, UID SEARCH, UID FETCH) against `imap.gmail.com:993` — command sequences in session (openssl s_client); no state changes.
- Report inspection via `docker exec postmanpat-postmanpat-rulesgen-1` on `/analyze-out/postmanpat-analyze-analyze-inbox.json` (report is root-owned 600, unreadable from host dir directly).
- Config: `/home/aaron/workspace/rocketman/postmanpat-config/{watch,cleanup-onetime,analysis,analysis-manual}.yml`.
- Code refs: `cli/watch.go:158-160,218`; `imap/internal/searches/manager.go:91-97,276-298`; `cli/cleanup.go`; `appconfig/config.go` (matcher schema, ignore lists).