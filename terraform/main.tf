resource "digitalocean_droplet" "web" {
  image    = "ubuntu-22-04-x64"
  name     = "docker-ubuntu-postmanpat"
  region   = "nyc3"
  size     = "s-1vcpu-1gb"
  ssh_keys = [var.ssh_fingerprint]
}

output "droplet_ip" {
  value = digitalocean_droplet.web.ipv4_address
}
