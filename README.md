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
- Server matchers: `age_window`, `folders`, `sender_substring`, `recipients`, `cc_substring`, `returnpath_substring` (matches `Return-Path` / MAIL FROM domain, e.g. Gmail “mailed-by”), `body_substring`, `replyto_substring`, `list_id_substring`, `seen` (true/false), `list_unsubscribe` (true/false).
- Client matchers: `subject_regex`, `body_regex`, `sender_regex`, `recipients_regex`, `cc_regex`, `returnpath_regex` (matches `Return-Path` / MAIL FROM domain, e.g. Gmail “mailed-by”), `replyto_regex`, `list_id_regex`, `recipient_tag_regex`, `list_unsubscribe` (true/false).

### Watch Test Mode

Use `watch --test` to run a one-off match check for a single rule and print the last N matches.

```bash
postmanpat watch --config /path/to/watch.yml --test "Rule Name" --limit 10 --mailbox "[Gmail]/All Mail"
```

Notes:
- `--test` takes a rule name from the config.
- `--limit` caps the number of matches returned (default 10).
- `--mailbox` overrides the mailbox to scan (default `INBOX`).

### Reporting and Checkpoint

- Reporting is enabled when `POSTMANPAT_WEBHOOK_URL` is set. No YAML config is needed for reporting.
- `checkpoint.path` is intended to store per-folder UID progress for long-running cleanup jobs. It is not currently used by `watch` or `cleanup`, but it is kept in the config for upcoming checkpointing support.

### Docker (Cleanup Cron)

This setup runs `postmanpat cleanup` every 15 minutes inside the container using cron.

1. **Initialize submodules**
   ```bash
   git submodule update --init --recursive
   ```

2. **Create a config file**
   - Place your cleanup config at `./config/config.yaml` (mounted to `/config/config.yaml` in the container).

3. **Set required environment variables**
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

4. **Run with docker-compose**
   ```bash
   docker compose up --build
   ```

5. **One-off cleanup run**
    ```bash
    docker compose run --rm postmanpat postmanpat cleanup --config /config/config_cleanup.yaml
    ```

**Overriding the cleanup config for a one-off run**

The bind-mounted cleanup config is set by the `POSTMANPAT_CLEANUP_CONFIG` env var (see `docker-compose.yml`). Prefer it over `-v` when running a one-off with a different config:

```bash
POSTMANPAT_CLEANUP_CONFIG=/path/to/cleanup-new.yml \
  docker compose run --rm postmanpat postmanpat cleanup --config /config/config_cleanup.yaml --dry-run
```

Notes:
- A missing host file becomes a root-owned directory: with `-v /host/file.yml:/config/config_cleanup.yaml`, the Docker daemon auto-creates a nonexistent host source **as a directory owned by root**. The container then mounts that directory over `/config/config_cleanup.yaml`, and the leftover directory must be removed with `sudo rmdir`. `docker compose run -v` behaves the same, and `:ro` does not prevent it (read-only applies only inside the container).
- `docker compose run -v` *adds* a mount instead of replacing the service's volume, so both the compose-defined config and the `-v` path target `/config/config_cleanup.yaml` and shadow each other.
- Only ad-hoc `compose run -v` mounts are exposed to this. The long-lived services mount `${POSTMANPAT_CLEANUP_CONFIG}` / `${POSTMANPAT_WATCH_CONFIG}` from `.env` (existing files under `/opt/docker/rocketman/postmanpat-config/`), and a compose-managed volume whose source already exists can never trigger auto-creation — which is why the watch config has never hit this problem.
- Beware the two rocketman checkouts: `bin/postmanpat-generate-rules.py` writes `--watch-out`/`--cleanup-out` relative to where it is run (e.g. `../rocketman/…` → `/opt/docker/rocketman/`), **not** your working clone (e.g. `/home/aaron/workspace/rocketman/`). Mounting the filename from the wrong tree means the file is "missing" even though it was generated — `ls` the exact mount path before running.
- Always create the host config file before running.

### Docker (Analyze)

Use a one-off container to run `postmanpat analyze`, which scans a mailbox using server matchers and writes a JSON report clustering senders and mailing lists (`list_lens`, `sender_unsub_lens`, `template_lens`, `recipient_tag_lens`). One report is written per rule; the mailbox scanned is the first entry in the rule's `server.folders`.

1. **Create an analyze config**
   - Rules must define `server` matchers only — `analyze` rejects rules with `client` matchers. `actions` are not required.
   - Example (`config/config_analyze.yaml`, gitignored):

     ```yaml
     rules:
       - name: "Analyze INBOX"
         server:
           age_window:
             min: "3d"
           folders:
             - "INBOX"
     ```

2. **Run a one-off analyze container**

   `analyze` writes the report to a temp file inside the container and prints its path. Set `TMPDIR` and mount a host directory at that path so the report survives `--rm` cleanup:

   ```bash
   mkdir -p analyze-out
   docker compose run --rm \
     -v ./config/config_analyze.yaml:/config/config_analyze.yaml:ro \
     -v ./analyze-out:/analyze-out \
     -e TMPDIR=/analyze-out \
     postmanpat postmanpat analyze --config /config/config_analyze.yaml
   ```

   - Flags: `--top` (max clusters per lens, default 100), `--examples` (max examples per field, default 20), `--min-count` (minimum cluster size, default 2).
   - Only the IMAP env vars are required; the S3 and webhook vars are unused by `analyze`.

3. **Run on rocketman (production)**

   Production configs live in the [rocketman repo](https://github.com/aaronromeo/rocketman/tree/main/postmanpat-config). From the postmanpat checkout on that host:

   ```bash
   sudo -u dockerops docker compose run --rm \
     -v /home/aaron/workspace/rocketman/postmanpat-config/config_analyze.yaml:/config/config_analyze.yaml:ro \
     -v /home/aaron/workspace/rocketman/postmanpat-config/analyze-out:/analyze-out \
     -e TMPDIR=/analyze-out \
     postmanpat postmanpat analyze --config /config/config_analyze.yaml
   ```

4. **Turn the report into rules**

   Feed the report into the helper script (requires PyYAML), which interactively generates watch and cleanup rules:

   ```bash
   python3 bin/postmanpat-generate-rules.py \
     --analyze analyze-out/postmanpat-analyze-*.json \
     --watch-out watch-new.yml \
     --cleanup-out cleanup-new.yml
   ```

   Review the generated rules, then merge them into the rocketman repo:
   - Watch rules → `postmanpat-config/watch.yml`
   - Cleanup rules → `postmanpat-config/cleanup-onetime.yml`

## Observability (OpenTelemetry + SigNoz)

postmanpat sends OpenTelemetry traces and metrics to any OTLP/gRPC backend.
Self-hosted SigNoz on the same machine is the supported target.

Set these in `.env` before `docker compose up`:

```bash
# Self-hosted SigNoz: collector gRPC endpoint (http:// scheme = plaintext)
OTEL_EXPORTER_OTLP_ENDPOINT=http://signoz-otel-collector:4317
OTEL_EXPORTER_OTLP_INSECURE=true
# SigNoz Cloud instead: endpoint ingest.<region>.signoz.cloud:443 over TLS
# with OTEL_EXPORTER_OTLP_HEADERS=signoz-ingestion-key=<key>
```

Telemetry is enabled only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Traces:

- `watch` — a `watch.cycle` span per new-mail batch, a `watch.message` span per
  email with one `watch.rule_evaluated` event per rule (`matched` attr), and a
  `watch.action` span per applied action.
- `cleanup` — one `cleanup.invocation` root span per run, with
  `cleanup.rule` and `cleanup.action` children and one `action.message_identified`
  event per matched email.
