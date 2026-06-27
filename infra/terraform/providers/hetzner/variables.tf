# --- Hetzner-specific ---
variable "hcloud_token" {
  type      = string
  sensitive = true
}

variable "os_image" {
  type    = string
  default = "ubuntu-22.04"
}

variable "network_zone" {
  # Hetzner network zone for the subnet. Must align with `location`:
  # eu-central -> nbg1/fsn1/hel1, us-east -> ash. Default matches location nbg1.
  type    = string
  default = "eu-central"
}

# --- Contract (identical across providers) ---
variable "cluster_name" {
  type = string
}

variable "ssh_pub_key" {
  type = string
}

variable "k3s_token" {
  type      = string
  sensitive = true
}

variable "control_plane" {
  type = object({
    server_type = string
    count       = number
  })
  default = { server_type = "cpx21", count = 1 }
}

variable "worker_pool" {
  type = object({
    server_type = string
    count       = number
    labels      = map(string)
    taints      = map(string)
  })
  default = {
    server_type = "cpx31"
    count       = 2
    labels      = {}
    taints      = {}
  }
}

variable "network_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "allowed_ssh_cidrs" {
  type    = list(string)
  default = []
}

variable "location" {
  type    = string
  default = "nbg1"
}
