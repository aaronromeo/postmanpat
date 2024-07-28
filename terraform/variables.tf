variable "do_token" {
  description = "DigitalOcean API token"
  type        = string
}

variable "ssh_fingerprint" {
  description = "SSH key fingerprint"
  type        = string
}

variable "pvt_key" {
  description = "Path to the SSH private key"
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
