# PostmanPat Deployment Guide

This document describes the automated deployment process for PostmanPat to a Dokku instance.

## Overview

The application uses **Git-based deployment** to a Dokku instance at `overachieverlabs.com`. Deployments are automatically triggered when code is pushed to the `main` branch.

## Architecture

- **Dokku Server**: `overachieverlabs.com`
- **App URL**: `http://postmanpat.overachieverlabs.com`
- **Process Types**: 
  - `web`: Main web server (port 3000)
  - `cron`: Background cron jobs
- **Deployment Method**: Git push to Dokku remote

## Deployment Process

### Automatic Deployment (Recommended)

1. **Push to main branch**: Any push to the `main` branch triggers automatic deployment
2. **GitHub Actions**: Runs the deployment workflow (`.github/workflows/deploy.yml`)
3. **Health Checks**: Verifies deployment success with HTTP health checks
4. **Rollback**: Automatically rolls back on deployment failure

### Manual Deployment

You can also deploy manually using the provided scripts:

```bash
# Setup the Dokku app (run once)
./scripts/setup-dokku-app.sh

# Deploy the application
./scripts/deploy.sh
```

## Required GitHub Secrets

Configure the following secrets in your GitHub repository settings:

### Required Secrets
- `DOKKU_SSH_PRIVATE_KEY`: SSH private key for accessing the Dokku server
- `IMAP_URL`: IMAP server URL (e.g., `imap.gmail.com:993`)
- `IMAP_USER`: IMAP username
- `IMAP_PASS`: IMAP password
- `DIGITALOCEAN_BUCKET_ACCESS_KEY`: DigitalOcean Spaces access key
- `DIGITALOCEAN_BUCKET_SECRET_KEY`: DigitalOcean Spaces secret key
- `UPTRACE_DSN`: Uptrace monitoring DSN

### Optional Variables
- `ENABLE_LETSENCRYPT`: Set to `true` to enable Let's Encrypt SSL (default: `false`)

## Local Development

For local development, use Docker Compose:

```bash
# Create environment file
cp .env.sample .env
# Edit .env with your local configuration

# Start services
docker-compose up

# Access the application
open http://localhost:3000
```

## Deployment Verification

The deployment process includes multiple verification steps:

1. **Dokku Status Check**: Verifies the app is running on Dokku
2. **HTTP Health Check**: Tests the application endpoint
3. **Rollback on Failure**: Automatically reverts to previous version if checks fail

## Troubleshooting

### Deployment Fails

1. Check GitHub Actions logs for detailed error messages
2. Verify all required secrets are configured
3. Test SSH connection to Dokku server manually

### App Not Accessible

1. Check Dokku app status: `ssh dokku-admin "dokku ps:report postmanpat"`
2. Check app logs: `ssh dokku-admin "dokku logs postmanpat"`
3. Verify domain configuration: `ssh dokku-admin "dokku domains:report postmanpat"`

### Manual Rollback

If automatic rollback fails, you can manually rollback:

```bash
ssh dokku-admin "dokku ps:rollback postmanpat"
```

## File Structure

```
.github/workflows/deploy.yml    # GitHub Actions deployment workflow
scripts/setup-dokku-app.sh      # Dokku app setup script
scripts/deploy.sh               # Deployment script
docker-compose.yml              # Local development setup
Procfile                        # Dokku process configuration
```

## Migration from Terraform

This deployment setup replaces the previous Terraform-based deployment. The old `terraform/` directory has been removed as it's no longer needed for the Git-based Dokku deployment approach.
