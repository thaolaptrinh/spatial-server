variable "zone_name"   { type = string }
variable "hostname"    { type = string }
variable "lb_dns_name" { type = string }
variable "lb_zone_id"  { type = string }

data "aws_route53_zone" "this" {
  name         = var.zone_name
  private_zone = false
}

resource "aws_route53_record" "gateway" {
  zone_id = data.aws_route53_zone.this.zone_id
  name    = var.hostname
  type    = "A"
  alias {
    name                   = var.lb_dns_name
    zone_id                = var.lb_zone_id
    evaluate_target_health = true
  }
}
