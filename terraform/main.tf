resource "digitalocean_droplet" "web" {
  image    = "ubuntu-22-04-x64"
  name     = "docker-ubuntu-postmanpat"
  region   = var.region
  size     = "s-1vcpu-1gb"
  ssh_keys = var.ssh_fingerprints

  tags = ["postmanpat"]

  user_data = <<-EOF
    #!/bin/bash
    echo '
      export IMAP_URL="${var.imap_url}"
      export IMAP_USER="${var.imap_user}"
      export IMAP_PASS="${var.imap_pass}"

      export DIGITALOCEAN_BUCKET_ACCESS_KEY="${var.digitalocean_bucket_access_key}"
      export DIGITALOCEAN_BUCKET_SECRET_KEY="${var.digitalocean_bucket_secret_key}"
      export DIGITALOCEAN_CONTAINER_REGISTRY_TOKEN="${var.digitalocean_container_registry_token}"
      export DIGITALOCEAN_USER="${var.digitalocean_user}"

    ' > /etc/profile.d/postmanpat.sh
  EOF
}

resource "null_resource" "provision" {
  triggers = {
    droplet_id           = digitalocean_droplet.web.id
    main_script_sha256   = filemd5("provision/main.sh")
    update_script_sha256 = filemd5("provision/update-script.sh")
    hooks_json_sha256    = filemd5("provision/hooks.json")
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
    source      = "provision/update-script.sh"
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
}

output "droplet_ip" {
  value = digitalocean_droplet.web.ipv4_address
}
