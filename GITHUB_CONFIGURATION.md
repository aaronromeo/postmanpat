# GitHub Configuration for Dokku Deployment

This document outlines all the required GitHub secrets and variables needed for the automated deployment workflow.

## Required GitHub Secrets

### 1. SSH Configuration

#### `DOKKU_SSH_PRIVATE_KEY` (Required)
- **Description**: SSH private key for authenticating with the Dokku server
- **Format**: OpenSSH private key format (recommended)
- **Example format**:
  ```
  -----BEGIN OPENSSH PRIVATE KEY-----
  b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAFwAAAAdzc2gtcn
  ... (key content) ...
  -----END OPENSSH PRIVATE KEY-----
  ```
- **Setup Instructions**:
  1. Generate a new SSH key pair: `ssh-keygen -t ed25519 -C "github-actions@postmanpat"`
  2. Add the public key to your Dokku server: `cat ~/.ssh/id_ed25519.pub >> /root/.ssh/authorized_keys`
  3. Copy the private key content to this GitHub secret

### 2. Application Configuration Secrets

#### `IMAP_URL` (Required for app functionality)
- **Description**: IMAP server URL for email processing
- **Format**: `imaps://imap.example.com:993` or similar
- **Example**: `imaps://imap.gmail.com:993`

#### `IMAP_USER` (Required for app functionality)
- **Description**: IMAP username/email for authentication
- **Format**: Email address or username
- **Example**: `your-email@example.com`

#### `IMAP_PASS` (Required for app functionality)
- **Description**: IMAP password or app-specific password
- **Format**: Plain text password
- **Security Note**: Use app-specific passwords when available (e.g., Gmail App Passwords)

#### `DIGITALOCEAN_BUCKET_ACCESS_KEY` (Required for app functionality)
- **Description**: DigitalOcean Spaces access key for file storage
- **Format**: Alphanumeric string
- **Example**: `AKIA1234567890EXAMPLE`

#### `DIGITALOCEAN_BUCKET_SECRET_KEY` (Required for app functionality)
- **Description**: DigitalOcean Spaces secret key for file storage
- **Format**: Base64-encoded string
- **Example**: `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`

#### `UPTRACE_DSN` (Optional - for monitoring)
- **Description**: Uptrace Data Source Name for application monitoring
- **Format**: URL with authentication
- **Example**: `https://token@api.uptrace.dev/project_id`

## GitHub Variables (Repository Variables)

### 1. SSL Configuration

#### `ENABLE_LETSENCRYPT` (Optional)
- **Description**: Enable automatic SSL certificate generation via Let's Encrypt
- **Values**: `true` or `false`
- **Default**: `false` (if not set)
- **Recommendation**: Set to `true` for production deployments

#### `LETSENCRYPT_EMAIL` (Optional)
- **Description**: Email address for Let's Encrypt certificate notifications
- **Format**: Valid email address
- **Default**: `letsencrypt@aaronromeo.com` (if not set)
- **Example**: `admin@yourdomain.com`

## How to Configure GitHub Secrets and Variables

### Setting up Secrets:

1. Go to your GitHub repository
2. Navigate to **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**
4. Add each secret with the exact name listed above
5. Paste the corresponding value

### Setting up Variables:

1. Go to your GitHub repository
2. Navigate to **Settings** → **Secrets and variables** → **Actions**
3. Click on the **Variables** tab
4. Click **New repository variable**
5. Add each variable with the exact name listed above
6. Set the corresponding value

## Troubleshooting

### SSH Key Issues:

1. **"error in libcrypto"**: 
   - Regenerate SSH key using: `ssh-keygen -t ed25519 -f ~/.ssh/dokku_key`
   - Ensure the private key is in OpenSSH format
   - Avoid RSA keys with older formats

2. **"Permission denied (publickey)"**:
   - Verify the public key is added to `/root/.ssh/authorized_keys` on the Dokku server
   - Check that the private key in GitHub secrets matches the public key on the server
   - Ensure the SSH key has proper permissions on the server

3. **Testing SSH Connection Locally**:
   ```bash
   # Test SSH connection to your Dokku server
   ssh -i ~/.ssh/your_private_key root@overachieverlabs.com "echo 'Connection successful'"
   ```

### Missing Secrets:

- The deployment workflow will validate all secrets and provide clear error messages
- Check the GitHub Actions logs for specific missing secrets
- Ensure secrets are set at the repository level (not organization level unless intended)

### Application Functionality:

- IMAP secrets are required for the email processing functionality
- DigitalOcean secrets are required for file storage
- UPTRACE_DSN is optional and only needed if you want application monitoring

## Security Best Practices

1. **Rotate SSH keys regularly** (every 6-12 months)
2. **Use app-specific passwords** for email accounts when available
3. **Limit SSH key permissions** on the server (only allow necessary commands)
4. **Monitor secret usage** in GitHub Actions logs
5. **Use environment-specific secrets** if deploying to multiple environments

## Verification

After setting up all secrets and variables, the deployment workflow will:

1. ✅ Validate all required secrets are present
2. ✅ Check SSH key format and validity
3. ✅ Test SSH connection to the Dokku server
4. ✅ Provide detailed error messages if any issues are found

The enhanced workflow includes comprehensive debugging information to help identify and resolve configuration issues quickly.
