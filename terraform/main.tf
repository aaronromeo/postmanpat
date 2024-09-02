resource "digitalocean_droplet" "web" {
  image    = "ubuntu-22-04-x64"
  name     = "docker-ubuntu-postmanpat"
  region   = var.region
  size     = "s-1vcpu-1gb"
  ssh_keys = var.SSH_FINGERPRINTS

  tags = ["postmanpat"]

  user_data = <<-EOF
    #!/bin/bash

    # Enable strict mode
    set -euxo pipefail

    echo '
      export IMAP_URL="${var.IMAP_URL}"
      export IMAP_USER="${var.IMAP_USER}"
      export IMAP_PASS="${var.IMAP_PASS}"

      export DIGITALOCEAN_BUCKET_ACCESS_KEY="${var.DIGITALOCEAN_BUCKET_ACCESS_KEY}"
      export DIGITALOCEAN_BUCKET_SECRET_KEY="${var.DIGITALOCEAN_BUCKET_SECRET_KEY}"
      export DIGITALOCEAN_CONTAINER_REGISTRY_TOKEN="${var.DIGITALOCEAN_CONTAINER_REGISTRY_TOKEN}"
      export DIGITALOCEAN_USER="${var.DIGITALOCEAN_USER}"

      export UPTRACE_DSN="${var.UPTRACE_DSN}"
    ' > /etc/profile.d/postmanpat.sh

    chmod +x /etc/profile.d/postmanpat.sh
  EOF
}


resource "digitalocean_domain" "app_domain" {
  name = var.DOMAIN
}

resource "digitalocean_record" "subdomain" {
  domain = digitalocean_domain.app_domain.name
  type   = "A"
  name   = var.SUBDOMAIN
  ttl    = 1800
  value  = digitalocean_droplet.web.ipv4_address

  depends_on = [digitalocean_droplet.web, digitalocean_domain.app_domain]
}

resource "null_resource" "provision" {
  triggers = {
    droplet_id            = digitalocean_droplet.web.id
    main_script_sha256    = filemd5("provision/main.sh")
    update_script_sha256  = filemd5("workingfiles/update-script.sh")
    hooks_json_sha256     = filemd5("provision/hooks.json")
    docker_compose_sha256 = filemd5("workingfiles/docker-compose.yml")
  }

  provisioner "file" {
    connection {
      type = "ssh"
      user = "root"

      # The key is password protected
      agent = true
      host  = digitalocean_droplet.web.ipv4_address
    }
    source      = "provision/main.sh"
    destination = "/tmp/provision.sh"
  }

  provisioner "file" {
    connection {
      type = "ssh"
      user = "root"

      # The key is password protected
      agent = true
      host  = digitalocean_droplet.web.ipv4_address
    }
    source      = "workingfiles/docker-compose.yml"
    destination = "/tmp/docker-compose.yml"
  }

  provisioner "file" {
    connection {
      type = "ssh"
      user = "root"

      # The key is password protected
      agent = true
      host  = digitalocean_droplet.web.ipv4_address
    }
    source      = "workingfiles/update-script.sh"
    destination = "/usr/local/bin/update-script.sh"
  }

  provisioner "file" {
    connection {
      type = "ssh"
      user = "root"

      # The key is password protected
      agent = true
      host  = digitalocean_droplet.web.ipv4_address
    }
    source      = "provision/hooks.json"
    destination = "/etc/webhook/hooks.json"
  }

  provisioner "remote-exec" {
    connection {
      type = "ssh"
      user = "root"

      # The key is password protected
      agent = true
      host  = digitalocean_droplet.web.ipv4_address
    }
    inline = [
      "chmod +x /tmp/provision.sh",
      "/tmp/provision.sh",
      "/usr/local/bin/update-script.sh"
    ]
  }

  depends_on = [digitalocean_droplet.web]
}

output "droplet_ip" {
  value = digitalocean_droplet.web.ipv4_address
}
