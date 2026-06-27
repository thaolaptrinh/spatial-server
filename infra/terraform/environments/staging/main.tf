# Pre-shared k3s token (cloud-agnostic, deterministic agent join)
resource "random_password" "k3s_token" {
  length  = 48
  special = false
}

# Cloud layer — change this `source` (and the token var) to switch cloud.
module "cloud" {
  source       = "../../providers/hetzner"
  cluster_name = "spatial-staging"
  ssh_pub_key  = var.ssh_pub_key
  k3s_token    = random_password.k3s_token.result
  hcloud_token = var.hcloud_token

  control_plane = { server_type = "cpx21", count = 1 }

  worker_pool = {
    server_type = "cpx31"
    count       = 2
    labels      = { workload = "game" }
    taints      = {}
  }

  allowed_ssh_cidrs = []
}

# DNS — Cloudflare (cloud-agnostic)
module "dns" {
  source               = "../../modules/dns"
  cloudflare_api_token = var.cloudflare_api_token
  zone_name            = var.dns_zone
  hostname             = "gateway.${var.dns_zone}"
  target               = module.cloud.load_balancer_endpoint
}
