variable "do_token" {
  description = "DigitalOcean API token"
  type        = string
}

variable "imap_pass" {
  description = "IMAP password"
  type        = string

}
variable "imap_url" {
  description = "IMAP URL"
  type        = string

}
variable "imap_user" {
  description = "IMAP user"
  type        = string

}
variable "digitalocean_bucket_access_key" {
  description = "Digitaocean bucket access key"
  type        = string

}
variable "digitalocean_bucket_secret_key" {
  description = "Digitaocean bucket secret key"
  type        = string
}

variable "digitalocean_container_registry_token" {
  description = "Digitaocean container registry token"
  type        = string
}

variable "digitalocean_user" {
  description = "Digitaocean user"
  type        = string
}

variable "domain" {
  description = "Domain name"
  type        = string
}

variable "subdomain" {
  description = "Subdomain name"
  type        = string
  default = "postman"
}

variable "pvt_key" {
  description = "Path to the SSH private key"
  type        = string
}

variable "pvt_key_file" {
  description = "Name of the SSH private key"
  type        = string
}

variable "spaces_access_key" {
  description = "Spaces access key"
  type        = string
}

variable "spaces_secret_key" {
  description = "Spaces secret key"
  type        = string
}

variable "ssh_fingerprints" {
  description = "SSH key fingerprints"
  type        = list(string)
}

variable "region" {
  description = "Digitalocean region"
  type        = string
  default     = "nyc3"
}
