# Stage 1 Requirements: Cleanup CLI

## Scope
- Provide a scheduled CLI command `postmanpat cleanup` to process all IMAP folders.
- Operate from a YAML config file (no database).
- Target Gmail and other IMAP providers; Gmail labels are treated as folders.
- Support large initial runs (tens of thousands of emails).

## CLI Behavior
- Scheduled via cron or similar.
- Support `--dry-run` and `--verbose` flags.
- Non-idempotent by nature (messages may be deleted).

## Configuration (YAML)
- Non-secret configuration only (rules, archive path templates, reporting channel, checkpoint path).
- Secrets and credentials are provided via environment variables.
- Rule list with server/client matchers, actions, and archive path template.
- Reporting configuration for Discord webhook (channel only).
- Checkpoint file location (separate from main config).

## Required Environment Variables
- IMAP: `POSTMANPAT_IMAP_HOST`, `POSTMANPAT_IMAP_PORT`, `POSTMANPAT_IMAP_USER`, `POSTMANPAT_IMAP_PASS`
- S3-compatible storage: `POSTMANPAT_S3_ENDPOINT`, `POSTMANPAT_S3_REGION`, `POSTMANPAT_S3_BUCKET`, `POSTMANPAT_S3_KEY`, `POSTMANPAT_S3_SECRET`
- Reporting: `POSTMANPAT_WEBHOOK_URL` (Discord)

## Rules and Actions
- Rules are ordered and **apply-all**.
- Each rule includes folder selection criteria.
- Matchers:
  - Server (IMAP search): age, sender/domain substrings, recipients, body substrings, reply-to substrings, list-id substrings, folders
  - Client (post-fetch): regex-capable matchers for subject/body/sender/recipient/reply-to/list-id
- Actions:
  - Archive to Spaces
  - Move to folder
  - Delete
- Action sequences (per rule):
  - archive then delete
  - move then delete
  - archive then move
  - delete

## Archival Format
- Store full raw message as `.eml`.
- Store decoded `.txt` and `.html` versions for search.
- Extract attachments.
- Naming:
  - Email: `<message-id>.eml`
  - Attachments: `<message-id>/<attachment-name>.<ext>`
- Archive destination path is defined by a template in each rule.

## Dedupe
- Deduplicate archived content across folders per run using a hash of the raw `.eml`.

## Checkpointing
- Store per-folder last processed UID in a separate local file.
- Allow reset/ignore to force a full rescan.

## Reporting
- Emit per-rule stats and error list to Slack or Discord via webhook.

## Implementation Decisions
- IMAP client: `github.com/emersion/go-imap/v2`.
- JMAP is out of scope for stage 1.

## Open Decisions
- Define archive path template variables and examples.
- Define checkpoint file schema and default location.
- Define reporting payload format for Slack/Discord.
