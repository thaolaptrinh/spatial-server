locals {
  # map(string) -> repeated k3s flags, e.g. "--node-label workload=game"
  node_label_args = join(" ", [for k, v in var.node_labels : "--node-label ${k}=${v}"])
  node_taint_args = join(" ", [for k, v in var.node_taints : "--node-taint ${k}=${v}"])

  role_file = var.role == "server" ? "k3s-server.yaml" : "k3s-agent.yaml"

  template_vars = {
    ssh_pub_key       = var.ssh_pub_key
    server_private_ip = var.server_private_ip
    k3s_token         = var.k3s_token
    node_label_args   = local.node_label_args
    node_taint_args   = local.node_taint_args
  }
}

data "cloudinit_config" "this" {
  gzip          = false
  base64_encode = false

  part {
    filename     = "common.yaml"
    content_type = "text/cloud-config"
    content = templatefile("${path.module}/../../../../cloud-init/common.yaml", {
      ssh_pub_key = var.ssh_pub_key
    })
  }

  part {
    filename     = local.role_file
    content_type = "text/cloud-config"
    merge_type   = "list(append)+dict(recurse_list,no_replace)+str()"
    content      = templatefile("${path.module}/../../../../cloud-init/${local.role_file}", local.template_vars)
  }
}
