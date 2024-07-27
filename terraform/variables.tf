variable "digitalocean_token" {
  description = "DigitalOcean API token"
  type        = string
}

variable "ssh_fingerprint" {
  description = "SSH key fingerprint"
  type        = string
}

variable "private_key_path" {
  description = "Path to the SSH private key"
  type        = string
}
