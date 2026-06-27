provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

data "cloudflare_zone" "this" {
  name = var.zone_name
}

locals {
  is_ipv4 = can(regex("^(?:[0-9]{1,3}\\.){3}[0-9]{1,3}$", var.target))
}

resource "cloudflare_record" "gateway" {
  zone_id = data.cloudflare_zone.this.id
  name    = var.hostname
  value   = var.target
  type    = local.is_ipv4 ? "A" : "CNAME"
  ttl     = var.proxied ? 1 : 300
  proxied = var.proxied
}
