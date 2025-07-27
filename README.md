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
   ```bash
   cp .env.sample .env
   # Edit .env with your configuration
   ```

3. **Start with Docker Compose**
   ```bash
   docker-compose up
   ```

4. **Access the application**
   - Web interface: http://localhost:3000

### Production Deployment

The application automatically deploys to `http://postmanpat.overachieverlabs.com` when code is pushed to the `main` branch.

**Setup Requirements:**
- Configure GitHub secrets and variables (see [GITHUB_CONFIGURATION.md](GITHUB_CONFIGURATION.md))
- Ensure SSH access to the Dokku server is properly configured

For detailed deployment information, see [DEPLOYMENT.md](DEPLOYMENT.md).

## Features

- **IMAP Email Processing**: Connects to any IMAP server to list and process mailboxes
- **Automated Email Archival**: Exports emails to structured storage (local filesystem or DigitalOcean Spaces)
- **Configurable Email Lifecycle**: Set mailboxes as exportable, deletable, or both with custom lifespans
- **Web Interface**: Monitor mailbox status and configurations via web dashboard
- **Cloud Storage Integration**: Seamless integration with DigitalOcean Spaces (S3-compatible)
- **Observability**: Built-in OpenTelemetry tracing and logging with Uptrace integration
- **Containerized Deployment**: Docker-based deployment with automated updates via Watchtower

## Architecture

PostmanPat operates in three main modes:

### 1. Mailbox Discovery (`mailboxnames`)
- Connects to IMAP server and discovers all available mailboxes
- Creates a configuration file (`workingfiles/mailboxlist.json`) with mailbox metadata
- Sets default properties (non-exportable, non-deletable) for new mailboxes

### 2. Email Processing (`reapmessages`)
- Reads mailbox configuration and processes emails based on settings:
  - **Exportable mailboxes**: Archives emails to cloud storage with metadata
  - **Deletable mailboxes**: Removes emails after optional export
  - **Lifespan-based filtering**: Only processes emails older than specified days
- Runs automatically via cron every 10 hours in production

### 3. Web Dashboard (`webserver`)
- Provides web interface on port 3000 for monitoring
- Displays mailbox configurations and status
- Serves static assets and provides REST endpoints

## Installation & Setup

### Prerequisites

- Go 1.21.5 or later
- Node.js 20+ (for web assets)
- IMAP email account credentials
- DigitalOcean Spaces credentials (for cloud storage)

### Environment Variables

Create a `.env` file with the following variables:

```bash
# IMAP Configuration
IMAP_URL=imap.example.com:993
IMAP_USER=your-email@example.com
IMAP_PASS=your-password

# DigitalOcean Spaces (S3-compatible storage)
DIGITALOCEAN_BUCKET_ACCESS_KEY=your-access-key
DIGITALOCEAN_BUCKET_SECRET_KEY=your-secret-key

# Observability (optional)
UPTRACE_DSN=your-uptrace-dsn
```

### Local Development

```bash
# Install dependencies
go mod download
npm install

# Build the application
make build

# Build web assets
make build-npm

# Run tests
make test

# Start web server locally
make webserver
```

## Usage

### Command Line Interface

PostmanPat provides three main commands:

#### 1. List Mailboxes
```bash
# Discover and configure mailboxes
./build/postmanpat mailboxnames
# or using alias
./build/postmanpat mn
```

#### 2. Process Messages
```bash
# Process emails based on mailbox configuration
./build/postmanpat reapmessages
# or using alias
./build/postmanpat re
```

#### 3. Start Web Server
```bash
# Start web dashboard on port 3000
./build/postmanpat webserver
# or using alias
./build/postmanpat ws
```

### Mailbox Configuration

After running `mailboxnames`, edit `workingfiles/mailboxlist.json` to configure mailbox behavior:

```json
{
  "INBOX": {
    "name": "INBOX",
    "delete": false,
    "export": true,
    "lifespan": 30
  },
  "Sent": {
    "name": "Sent",
    "delete": true,
    "export": true,
    "lifespan": 90
  }
}
```

- **`export`**: Whether to archive emails to cloud storage
- **`delete`**: Whether to delete emails after processing
- **`lifespan`**: Only process emails older than this many days

## Testing

### Run All Tests
```bash
make test
```

### Generate Coverage Report
```bash
# Run tests with coverage
go test -v ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...

# Generate HTML coverage report
go tool cover -html=./cover.out -o ./cover.html

# View coverage summary
go tool cover -func=./cover.out
```

### Current Test Coverage
- **Overall**: ~57% statement coverage
- **IMAP Manager**: 75.9% coverage
- **Mailbox Processing**: 62.5% coverage
- **CLI Commands**: 0% coverage (needs improvement)
- **HTTP Handlers**: 0% coverage (needs improvement)

## Deployment

### Docker Compose (Production)

```bash
# Deploy with Docker Compose
docker-compose up -d
```

The production deployment includes:
- **Cron container**: Runs `reapmessages` every 10 hours
- **Web container**: Serves dashboard on port 3000
- **Watchtower**: Automatically updates containers

### Terraform (DigitalOcean)

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

Deploys complete infrastructure including:
- DigitalOcean droplet
- Domain configuration
- Automated provisioning

### Deploying Applications to Dokku

1. Create a new application:
   ```bash
   ssh dokku-admin "dokku apps:create postmanpat"
   ```

2. Set up your local Git repository:
   ```bash
   # In your application directory
   git remote add dokku root@overachieverlabs.com:postmanpat
   ```

3. Deploy your application:
   ```bash
   git push dokku main
   ```

4. Set up SSL with Let's Encrypt:
   ```bash
   ssh dokku-admin "dokku letsencrypt:enable postmanpat"
   ```

5. Access your application at:
   - http://postmanpat.overachieverlabs.com
   - https://postmanpat.overachieverlabs.com (after enabling Let's Encrypt)

### Common Dokku Commands

```bash
# List all applications
ssh dokku-admin "dokku apps:list"

# Check application status
ssh dokku-admin "dokku ps:report postmanpat"

# View application logs
ssh dokku-admin "dokku logs postmanpat -t"

# Set environment variables
ssh dokku-admin "dokku config:set postmanpat KEY=VALUE"

# List environment variables
ssh dokku-admin "dokku config postmanpat"

# Restart an application
ssh dokku-admin "dokku ps:restart postmanpat"

# Create a PostgreSQL database (if needed)
ssh dokku-admin "dokku postgres:create postmanpat-db"

# Link a database to an application (if needed)
ssh dokku-admin "dokku postgres:link postmanpat-db postmanpat"
```

For more information, see the [Dokku documentation](https://dokku.com/docs/).

## Sample Workflow

1. **Initial Setup**:
   ```bash
   # Discover mailboxes
   ./build/postmanpat mailboxnames
   ```

2. **Configure Mailboxes**:
   Edit `workingfiles/mailboxlist.json` to set export/delete policies

3. **Process Emails**:
   ```bash
   # Manual processing
   ./build/postmanpat reapmessages
   ```

4. **Monitor via Web**:
   ```bash
   # Start dashboard
   ./build/postmanpat webserver
   # Visit http://localhost:3000
   ```

## File Structure

```
postmanpat/
├── cmd/postmanpat/          # Main application entry point
├── pkg/
│   ├── models/
│   │   ├── imapmanager/     # IMAP connection management
│   │   └── mailbox/         # Email processing logic
│   ├── utils/               # Storage and utility functions
│   ├── base/                # Common types and constants
│   ├── commands/            # CLI command implementations
│   ├── repositories/        # Data access layer
│   ├── services/            # Business logic services
│   ├── testutil/            # Test utilities and helpers
│   └── mock/                # Test mocks and helpers
├── handlers/                # HTTP request handlers
├── views/                   # HTML templates
│   ├── layouts/             # Layout templates
│   ├── mailboxes/           # Mailbox-specific views
│   └── partials/            # Reusable template components
├── public/                  # Static web assets
│   └── assets/              # Compiled CSS/JS assets
├── assets/                  # Source assets (CSS, JS)
├── scripts/                 # Deployment and utility scripts
├── examples/                # Usage examples and documentation
├── workingfiles/            # Runtime configuration and data
├── .github/workflows/       # GitHub Actions CI/CD
├── .devcontainer/           # Development container configuration
├── docker-compose.yml       # Local development setup
├── Dockerfile.ws            # Web server container
├── Dockerfile.cron          # Cron job container
├── Procfile                 # Dokku process definitions
├── Makefile                 # Build and development commands
└── README.md                # This file
```

## Environment Variables

See `.env.sample` for required environment variables. Key variables include:

- **IMAP_URL**: IMAP server URL and port
- **IMAP_USER**: IMAP username
- **IMAP_PASS**: IMAP password
- **DIGITALOCEAN_BUCKET_ACCESS_KEY**: DigitalOcean Spaces access key
- **DIGITALOCEAN_BUCKET_SECRET_KEY**: DigitalOcean Spaces secret key
- **UPTRACE_DSN**: Uptrace observability endpoint (optional)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run the test suite: `make test`
6. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
