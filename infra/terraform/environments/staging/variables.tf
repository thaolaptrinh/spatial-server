variable "ssh_pub_key" { type = string }
variable "db_password" { type = string; sensitive = true }
variable "dns_zone"    { type = string }
variable "lb_dns_name" { type = string }
variable "lb_zone_id"  { type = string }
