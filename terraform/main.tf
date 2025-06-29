# Droplet resource and associated provisioning resources have been removed
# as this is now handled by the Dokku instance

output "droplet_ip" {
  value = var.DOKKU_DOMAIN
}

output "app_url" {
  value = "http://${var.APP_NAME}.${var.DOKKU_DOMAIN}"
}

output "git_remote_setup" {
  value = <<-EOT
    # To set up the git remote for deployment:
    git remote add dokku dokku@${var.DOKKU_DOMAIN}:${var.APP_NAME}

    # To deploy your code:
    git push dokku main:master
  EOT
}

# Configure the Dokku app using domain
resource "null_resource" "dokku_app" {
  triggers = {
    app_name = var.APP_NAME
    dokku_domain = var.DOKKU_DOMAIN
    # Add environment variables to triggers to update when they change
    env_hash = sha256(jsonencode({
      IMAP_URL = var.IMAP_URL
      IMAP_USER = var.IMAP_USER
      IMAP_PASS = var.IMAP_PASS
      DIGITALOCEAN_BUCKET_ACCESS_KEY = var.DIGITALOCEAN_BUCKET_ACCESS_KEY
      DIGITALOCEAN_BUCKET_SECRET_KEY = var.DIGITALOCEAN_BUCKET_SECRET_KEY
      UPTRACE_DSN = var.UPTRACE_DSN
    }))
  }

  connection {
    type  = "ssh"
    user  = "root"
    agent = true
    host  = var.DOKKU_DOMAIN
  }

  # Create and configure the app
  provisioner "remote-exec" {
    inline = [
      # Create app if it doesn't exist
      "dokku apps:exists ${var.APP_NAME} || dokku apps:create ${var.APP_NAME}",

      # Configure environment variables
      "dokku config:set --no-restart ${var.APP_NAME} IMAP_URL='${var.IMAP_URL}'",
      "dokku config:set --no-restart ${var.APP_NAME} IMAP_USER='${var.IMAP_USER}'",
      "dokku config:set --no-restart ${var.APP_NAME} IMAP_PASS='${var.IMAP_PASS}'",
      "dokku config:set --no-restart ${var.APP_NAME} DIGITALOCEAN_BUCKET_ACCESS_KEY='${var.DIGITALOCEAN_BUCKET_ACCESS_KEY}'",
      "dokku config:set --no-restart ${var.APP_NAME} DIGITALOCEAN_BUCKET_SECRET_KEY='${var.DIGITALOCEAN_BUCKET_SECRET_KEY}'",
      "dokku config:set --no-restart ${var.APP_NAME} UPTRACE_DSN='${var.UPTRACE_DSN}'",

      # Configure domains
      "dokku domains:add ${var.APP_NAME} ${var.APP_NAME}.${var.DOKKU_DOMAIN}",

      # Configure ports
      "dokku ports:set ${var.APP_NAME} http:80:3000",

      # Enable letsencrypt if domain is provided
      "if [[ '${var.ENABLE_LETSENCRYPT}' == 'true' && '${var.DOKKU_DOMAIN}' != *\".\"* ]]; then dokku letsencrypt:enable ${var.APP_NAME}; fi"
    ]
  }
}
