variable "DO_TOKEN" {
  description = "DigitalOcean API token (needed for state management)"
  type        = string
  sensitive   = true
}

variable "DOKKU_DOMAIN" {
  description = "Domain or IP address of the existing Dokku server"
  type        = string
}

variable "APP_NAME" {
  description = "Name of the Dokku application"
  type        = string
  default     = "postmanpat"
}

variable "ENABLE_LETSENCRYPT" {
  description = "Enable Let's Encrypt SSL for the app"
  type        = bool
  default     = false
}

# Environment variables
variable "IMAP_URL" {
  description = "IMAP server URL"
  type        = string
  sensitive   = true
}

variable "IMAP_USER" {
  description = "IMAP username"
  type        = string
  sensitive   = true
}

variable "IMAP_PASS" {
  description = "IMAP password"
  type        = string
  sensitive   = true
}

variable "DIGITALOCEAN_BUCKET_ACCESS_KEY" {
  description = "DigitalOcean Spaces access key"
  type        = string
  sensitive   = true
}

variable "DIGITALOCEAN_BUCKET_SECRET_KEY" {
  description = "DigitalOcean Spaces secret key"
  type        = string
  sensitive   = true
}

variable "UPTRACE_DSN" {
  description = "Uptrace DSN for monitoring"
  type        = string
  sensitive   = true
}

# The following variables have been removed as they're no longer needed
# since the droplet resource is now handled by the Dokku instance:
# - SSH_FINGERPRINTS
# - region
# - pvt_key_file
# - DIGITALOCEAN_CONTAINER_REGISTRY_TOKEN
# - DIGITALOCEAN_USER
