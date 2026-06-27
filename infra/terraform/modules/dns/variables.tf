variable "cloudflare_api_token" {
  type      = string
  sensitive = true
}

variable "zone_name" {
  type = string
}

variable "hostname" {
  type = string
}

# LB hostname (CNAME) or IP (A) from the provider module
variable "target" {
  type = string
}

variable "proxied" {
  type    = bool
  default = false
}
