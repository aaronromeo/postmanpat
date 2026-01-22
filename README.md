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
   Edit .env with your configuration. The env vars `POSTMANPAT_IMAP_HOST`, `POSTMANPAT_IMAP_PORT`, `POSTMANPAT_IMAP_USER`, `POSTMANPAT_IMAP_PASS`, `POSTMANPAT_DO_ENDPOINT`, `POSTMANPAT_DO_REGION`, `POSTMANPAT_DO_BUCKET`, `POSTMANPAT_DO_KEY`, `POSTMANPAT_DO_SECRET`, `POSTMANPAT_WEBHOOK_URL` are required.

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
   An example of how to run the cleanup
   `$ bin/postmanpat cleanup --config internal/config/config_onetime_cleanup.yaml`

   An example of how to run the analyze script
   ``

