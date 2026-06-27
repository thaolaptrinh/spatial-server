provider "hcloud" {
  token = var.hcloud_token
}

locals {
  # control-plane nodes start at <network>.0.2
  cp_ip_offset = 2
  # worker nodes start at <network>.0.32
  worker_ip_base = 32
}

# ---------------------------------------------------------------------------
# SSH key
# ---------------------------------------------------------------------------
resource "hcloud_ssh_key" "this" {
  name       = "${var.cluster_name}-key"
  public_key = var.ssh_pub_key
}

# ---------------------------------------------------------------------------
# Private network + subnet
# ---------------------------------------------------------------------------
resource "hcloud_network" "this" {
  name     = "${var.cluster_name}-net"
  ip_range = var.network_cidr
}

resource "hcloud_network_subnet" "this" {
  network_id   = hcloud_network.this.id
  type         = "cloud"
  ip_range     = var.network_cidr
  network_zone = var.network_zone
}

# ---------------------------------------------------------------------------
# Firewall: 80/443 public, SSH from allow-list, everything inside the network
# ---------------------------------------------------------------------------
resource "hcloud_firewall" "this" {
  name = "${var.cluster_name}-fw"

  dynamic "rule" {
    for_each = length(var.allowed_ssh_cidrs) > 0 ? [1] : []
    content {
      direction  = "in"
      protocol   = "tcp"
      port       = "22"
      source_ips = var.allowed_ssh_cidrs
    }
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "80"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    source_ips = [var.network_cidr]
  }

  rule {
    direction  = "in"
    protocol   = "udp"
    source_ips = [var.network_cidr]
  }

  rule {
    direction  = "in"
    protocol   = "icmp"
    source_ips = [var.network_cidr]
  }

  rule {
    direction       = "out"
    protocol        = "tcp"
    destination_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction       = "out"
    protocol        = "udp"
    destination_ips = ["0.0.0.0/0", "::/0"]
  }
}

# ---------------------------------------------------------------------------
# Control-plane node(s)
# ---------------------------------------------------------------------------
locals {
  control_plane_private_ip = cidrhost(var.network_cidr, local.cp_ip_offset)
}

module "control_plane_cloudinit" {
  source            = "../shared/cloudinit"
  role              = "server"
  ssh_pub_key       = var.ssh_pub_key
  server_private_ip = local.control_plane_private_ip
  k3s_token         = var.k3s_token
}

resource "hcloud_server" "control_plane" {
  count        = var.control_plane.count
  name         = "${var.cluster_name}-cp-${count.index + 1}"
  image        = var.os_image
  server_type  = var.control_plane.server_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.this.name]
  firewall_ids = [hcloud_firewall.this.id]
  user_data    = module.control_plane_cloudinit.rendered
  # Required by hcloud docs: the server's network attachment must wait for the subnet
  depends_on = [hcloud_network_subnet.this]

  network {
    network_id = hcloud_network.this.id
    ip         = cidrhost(var.network_cidr, local.cp_ip_offset + count.index)
  }
}

# ---------------------------------------------------------------------------
# Worker node pool (fixed count; labels/taints flow in via the shared module)
# ---------------------------------------------------------------------------
module "worker_cloudinit" {
  source            = "../shared/cloudinit"
  role              = "agent"
  ssh_pub_key       = var.ssh_pub_key
  server_private_ip = local.control_plane_private_ip
  k3s_token         = var.k3s_token
  node_labels       = var.worker_pool.labels
  node_taints       = var.worker_pool.taints
}

resource "hcloud_server" "worker" {
  count        = var.worker_pool.count
  name         = "${var.cluster_name}-worker-${count.index + 1}"
  image        = var.os_image
  server_type  = var.worker_pool.server_type
  location     = var.location
  ssh_keys     = [hcloud_ssh_key.this.name]
  firewall_ids = [hcloud_firewall.this.id]
  user_data    = module.worker_cloudinit.rendered
  depends_on   = [hcloud_network_subnet.this]

  network {
    network_id = hcloud_network.this.id
    ip         = cidrhost(var.network_cidr, local.worker_ip_base + count.index)
  }
}

# ---------------------------------------------------------------------------
# Public load balancer → control-plane :80/:443 (Traefik ingress ONLY).
# Agents join via the private IP (6443), NOT through the LB.
# ---------------------------------------------------------------------------
resource "hcloud_load_balancer" "this" {
  name               = "${var.cluster_name}-lb"
  load_balancer_type = "lb11"
  location           = var.location
}

resource "hcloud_load_balancer_network" "this" {
  load_balancer_id = hcloud_load_balancer.this.id
  network_id       = hcloud_network.this.id
  ip               = cidrhost(var.network_cidr, 3)
}

resource "hcloud_load_balancer_target" "control_plane" {
  count            = var.control_plane.count
  type             = "server"
  load_balancer_id = hcloud_load_balancer.this.id
  server_id        = hcloud_server.control_plane[count.index].id
}

resource "hcloud_load_balancer_service" "http" {
  load_balancer_id = hcloud_load_balancer.this.id
  protocol         = "tcp"
  listen_port      = 80
  destination_port = 80
}

resource "hcloud_load_balancer_service" "https" {
  load_balancer_id = hcloud_load_balancer.this.id
  protocol         = "tcp"
  listen_port      = 443
  destination_port = 443
}
