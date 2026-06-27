# --- Cloud-agnostic ---
variable "ssh_pub_key" {
  type = string
}

variable "dns_zone" {
  type = string
}

# --- Cloud credentials (the only lines that change when switching cloud) ---
variable "hcloud_token" {
  type      = string
  sensitive = true
}

variable "cloudflare_api_token" {
  type      = string
  sensitive = true
}
