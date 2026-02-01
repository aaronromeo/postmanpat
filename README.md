# PostmanPat

PostmanPat is a Go-based email processing and archival system that connects to IMAP email servers to automatically manage email messages. It provides automated email archival, cleanup, and a web interface for monitoring mailbox operations.

## Quick Start

### Local Development

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd postmanpat
   ```

2. **Set up environment**
   Edit .env with your configuration. The env vars `POSTMANPAT_IMAP_HOST`, `POSTMANPAT_IMAP_PORT`, `POSTMANPAT_IMAP_USER`, `POSTMANPAT_IMAP_PASS`, `POSTMANPAT_S3_ENDPOINT`, `POSTMANPAT_S3_REGION`, `POSTMANPAT_S3_BUCKET`, `POSTMANPAT_S3_KEY`, `POSTMANPAT_S3_SECRET`, `POSTMANPAT_WEBHOOK_URL` are required.

   ```bash
   cp .env.sample .env
   ```

3. **Go mod tidy if needed**
   ```bash
   go mod tidy
   ```

4. **Build the app**
   ```bash
   go build -o bin ./...
   ```

5. **Base case**
   ```bash
    $ bin/postmanpat
    postmanpat manages email cleanup and archiving

    Usage:
    postmanpat [command]

    Available Commands:
    cleanup     Process IMAP folders based on configured rules
    completion  Generate the autocompletion script for the specified shell
    help        Help about any command

    Flags:
    -h, --help   help for postmanpat

    Use "postmanpat [command] --help" for more information about a command.
    ```

### Rule Matchers

Rules now separate server-side and client-side matchers:

```yaml
rules:
  - name: "Example"
    server:
      age_window:
        min: "24h" # at least 24 hours old
        max: "7d"  # at most 7 days old
      folders: ["INBOX"]
      sender_substring:
        - "example.com"
    client:
      subject_regex:
        - "(?i)welcome"
```

- `server` matchers are used for IMAP SEARCH (substring/age/folder).
- `client` matchers are reserved for post-fetch regex filtering (used by `watch`).
- `age_window` uses IMAP INTERNALDATE, not the message `Date:` header.
- `age_window` defines a bounded range: `min` is the minimum age (older than), `max` is the maximum age (newer than).

### Reporting and Checkpoint

These config blocks are not required for client matchers (watch) today, but they remain part of the config format.

- `reporting.channel` identifies the reporting target (for example, `discord` or `slack`). It is reserved for future reporting output; the webhook URL is still provided via `POSTMANPAT_WEBHOOK_URL`.
- `checkpoint.path` is intended to store per-folder UID progress for long-running cleanup jobs. It is not currently used by `watch` or `cleanup`, but it is kept in the config for upcoming checkpointing support.

### Docker (Cleanup Cron)

This setup runs `postmanpat cleanup` every 15 minutes inside the container using cron.

1. **Create a config file**
   - Place your cleanup config at `./config/config.yaml` (mounted to `/config/config.yaml` in the container).

2. **Set required environment variables**
   - Required IMAP and reporting/Spaces env vars:
     - `POSTMANPAT_IMAP_HOST`
     - `POSTMANPAT_IMAP_PORT`
     - `POSTMANPAT_IMAP_USER`
     - `POSTMANPAT_IMAP_PASS`
     - `POSTMANPAT_S3_ENDPOINT`
     - `POSTMANPAT_S3_REGION`
     - `POSTMANPAT_S3_BUCKET`
     - `POSTMANPAT_S3_KEY`
     - `POSTMANPAT_S3_SECRET`
     - `POSTMANPAT_WEBHOOK_URL`
   - The container also expects:
     - `POSTMANPAT_CONFIG` (set by compose to `/config/config.yaml`)

3. **Run with docker-compose**
   ```bash
   docker compose up --build
   ```
