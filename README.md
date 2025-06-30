# PostmanPat

A Go-based email processing application with web interface and cron job capabilities.

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

For detailed deployment information, see [DEPLOYMENT.md](DEPLOYMENT.md).

## Architecture

- **Web Server**: Go-based web application (port 3000)
- **Cron Jobs**: Background email processing
- **Storage**: DigitalOcean Spaces for file storage
- **Monitoring**: Uptrace integration

## Environment Variables

See `.env.sample` for required environment variables.

## Sample Usage

### Export syntax

```text
IMAP_FOLDER="AFolderNamedWork" make run
```
