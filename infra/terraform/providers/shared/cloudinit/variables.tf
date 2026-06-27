variable "role" {
  type = string
  validation {
    condition     = contains(["server", "agent"], var.role)
    error_message = "role must be \"server\" or \"agent\"."
  }
}

variable "ssh_pub_key" {
  type = string
}

variable "server_private_ip" {
  type = string
}

variable "k3s_token" {
  type      = string
  sensitive = true
}

variable "node_labels" {
  type    = map(string)
  default = {}
}

variable "node_taints" {
  type    = map(string)
  default = {}
}
