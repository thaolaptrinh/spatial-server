module "providers" { source = "../../providers" }

module "vpc" {
  source = "../../modules/vpc"
  name   = "spatial-staging"
}

module "k3s" {
  source       = "../../modules/k3s"
  subnet_id    = module.vpc.private_subnets[0]
  sg_id        = module.vpc.k3s_sg_id
  ssh_pub_key  = var.ssh_pub_key
}

module "agents" {
  source             = "../../modules/node_pool"
  name               = "spatial-staging-agents"
  subnet_ids         = module.vpc.private_subnets
  sg_id              = module.vpc.k3s_sg_id
  server_private_ip  = module.k3s.server_private_ip
  k3s_token          = "WIRE_FROM_K3S_OUTPUT"
  ssh_pub_key        = var.ssh_pub_key
}

module "rds" {
  source         = "../../modules/rds"
  subnet_ids     = module.vpc.private_subnets
  vpc_id         = module.vpc.vpc_id
  allowed_sg_ids = [module.vpc.k3s_sg_id]
  db_password    = var.db_password
}

module "redis" {
  source         = "../../modules/elasticache"
  subnet_ids     = module.vpc.private_subnets
  vpc_id         = module.vpc.vpc_id
  allowed_sg_ids = [module.vpc.k3s_sg_id]
}

module "dns" {
  source       = "../../modules/dns"
  zone_name    = var.dns_zone
  hostname     = "gateway.${var.dns_zone}"
  lb_dns_name  = var.lb_dns_name
  lb_zone_id   = var.lb_zone_id
}
