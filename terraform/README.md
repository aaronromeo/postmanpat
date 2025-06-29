# Terraform Configuration for PostmanPat on Dokku

This directory contains Terraform configurations to deploy PostmanPat to an existing Dokku instance created by the Morph project.

## Prerequisites

- [Terraform](https://www.terraform.io/downloads.html) installed
- SSH access to the Dokku server
- Existing Dokku server (created by the Morph project)
- DigitalOcean API token (DO_TOKEN) for state management
- DigitalOcean Spaces credentials for the S3 backend (these are AWS-compatible credentials)

## Setup

1. **Create a `terraform.tfvars` file** with your configuration values:

```
# DigitalOcean configuration (needed for state management)
DO_TOKEN = "dop_v1_xxxxxxxxxxxxxxxxxxxx"

# Dokku configuration
DOKKU_DOMAIN = "dokku.example.com"  # Domain or IP of your Dokku server

# Application configuration
APP_NAME = "postmanpat"
ENABLE_LETSENCRYPT = false

# Application environment variables
IMAP_URL = "imap.example.com:993"
IMAP_USER = "user@example.com"
IMAP_PASS = "password"
DIGITALOCEAN_BUCKET_ACCESS_KEY = "your_spaces_access_key"
DIGITALOCEAN_BUCKET_SECRET_KEY = "your_spaces_secret_key"
UPTRACE_DSN = "https://token@api.uptrace.dev/project_id"
```

2. **Set up authentication**:

```bash
# For S3 backend (DigitalOcean Spaces)
# These AWS-compatible credentials are used to authenticate with DigitalOcean Spaces
# They are different from the DigitalOcean API token (DO_TOKEN)
export AWS_ACCESS_KEY_ID=your_spaces_access_key
export AWS_SECRET_ACCESS_KEY=your_spaces_secret_key

# Ensure your SSH key is added to the SSH agent
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/your_private_key
```

## Usage

Initialize Terraform:

```bash
terraform init
```

Plan your changes:

```bash
terraform plan -var-file="terraform.tfvars"
```

Apply the changes:

```bash
terraform apply -var-file="terraform.tfvars"
```

## Deployment

After Terraform has set up the Dokku app, you can deploy your code using Git:

1. **Add the Dokku remote to your Git repository**:

```bash
git remote add dokku dokku@your-dokku-domain:your-app-name
```

2. **Ensure you have a Procfile** in the root of your repository:

```
web: /app/build/postmanpat ws
cron: cron -f
```

3. **Deploy your code**:

```bash
git push dokku main:master
```

## Notes

- This configuration uses an existing Dokku instance created by the Morph project
- The deployment lifecycle is handled by a Procfile rather than docker-compose.yml
- Dokku will automatically build and deploy your application based on the Procfile
- All data is stored in DigitalOcean Block Storage, so no persistent storage is configured

## Transitioning from Direct DigitalOcean Management

If you're transitioning from directly managing DigitalOcean resources to using an existing Dokku instance, you may need to remove the old resources from your Terraform state:

```bash
# List the resources in your state
terraform state list

# Remove the DigitalOcean droplet from the state
terraform state rm digitalocean_droplet.postmanpat

# Remove any other resources that are no longer in your configuration
terraform state rm null_resource.source_files
terraform state rm null_resource.provision
```

This will allow Terraform to manage only the resources defined in your current configuration without trying to access the old DigitalOcean resources.
