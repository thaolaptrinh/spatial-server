output "control_plane_private_ips" {
  value = hcloud_server.control_plane[*].network[0].ip
}

output "control_plane_public_ips" {
  value = hcloud_server.control_plane[*].ipv4_address
}

output "worker_private_ips" {
  value = hcloud_server.worker[*].network[0].ip
}

output "load_balancer_endpoint" {
  # Hetzner LBs are IP-only (no managed hostname); use the IPv4. On clouds that
  # expose a hostname (e.g. AWS ALB DNS name) this output holds that instead.
  value = hcloud_load_balancer.this.ipv4
}

output "load_balancer_ip" {
  value = hcloud_load_balancer.this.ipv4
}

output "network_cidr" {
  value = var.network_cidr
}
