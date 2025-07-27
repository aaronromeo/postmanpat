# GitHub Repository Setup

This document describes how to configure your GitHub repository for automatic deployment to Dokku.

## Required GitHub Secrets

Navigate to your GitHub repository → Settings → Secrets and variables → Actions, then add the following secrets:

### 1. SSH Access
- **Name**: `DOKKU_SSH_PRIVATE_KEY`
- **Value**: Your SSH private key content (the content of `~/.ssh/dokku_key`)
- **Description**: SSH private key for accessing the Dokku server

### 2. IMAP Configuration
- **Name**: `IMAP_URL`
- **Value**: `imap.gmail.com:993` (or your IMAP server)
- **Description**: IMAP server URL and port

- **Name**: `IMAP_USER`
- **Value**: Your email address
- **Description**: IMAP username/email

- **Name**: `IMAP_PASS`
- **Value**: Your email password or app-specific password
- **Description**: IMAP password

### 3. DigitalOcean Spaces
- **Name**: `DIGITALOCEAN_BUCKET_ACCESS_KEY`
- **Value**: Your DigitalOcean Spaces access key
- **Description**: Access key for DigitalOcean Spaces

- **Name**: `DIGITALOCEAN_BUCKET_SECRET_KEY`
- **Value**: Your DigitalOcean Spaces secret key
- **Description**: Secret key for DigitalOcean Spaces

### 4. Monitoring
- **Name**: `UPTRACE_DSN`
- **Value**: Your Uptrace DSN URL
- **Description**: Uptrace monitoring configuration

## Optional GitHub Variables

Navigate to your GitHub repository → Settings → Secrets and variables → Actions → Variables tab:

- **Name**: `ENABLE_LETSENCRYPT`
- **Value**: `true` or `false`
- **Description**: Enable Let's Encrypt SSL certificates

## How to Get Your SSH Private Key

1. **Display your private key**:
   ```bash
   cat ~/.ssh/dokku_key
   ```

2. **Copy the entire output** (including the `-----BEGIN` and `-----END` lines)

3. **Paste it as the value** for the `DOKKU_SSH_PRIVATE_KEY` secret

## Testing the Setup

1. **Push to main branch**: Any push to `main` will trigger deployment
2. **Check Actions tab**: Monitor the deployment progress in GitHub Actions
3. **Verify deployment**: Check that the app is accessible at `http://postmanpat.overachieverlabs.com`

## Troubleshooting

### SSH Key Issues
- Ensure the SSH key has the correct permissions on the Dokku server
- Test SSH access manually: `ssh -i ~/.ssh/dokku_key root@overachieverlabs.com`

### Missing Secrets
- All required secrets must be configured for deployment to work
- Check the GitHub Actions logs for specific error messages

### Deployment Failures
- Check the GitHub Actions workflow logs for detailed error information
- Verify that the Dokku server is accessible and running
- Ensure all environment variables are properly configured

## Security Notes

- Never commit sensitive information (passwords, keys) to the repository
- Use GitHub Secrets for all sensitive configuration
- Regularly rotate SSH keys and access tokens
- Monitor deployment logs for any security issues
