# Multi-cloud Terraform Migration Implementation Plan (rev 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace AWS-specific Terraform with a cloud-agnostic provider abstraction (Hetzner first), so switching an environment's cloud (Hetzner → Sakura/AWS) is a localized change.

**Architecture:** K3s-on-VMs + cloud-init + Helm is already cloud-agnostic; only the IaaS Terraform layer is cloud-specific. Each cloud lives in `infra/terraform/providers/<cloud>/` implementing one shared contract (identical inputs/outputs). Cloud-init rendering is shared in `providers/shared/cloudinit/`. Shared layers (cloud-init, Cloudflare DNS module, HCP Terraform state, CloudNativePG Postgres, Bitnami Redis Helm charts) are reused verbatim across clouds.

**Tech Stack:** Terraform ≥1.7, `hetznercloud/hcloud`, `hashicorp/cloudinit`, `cloudflare/cloudflare`, `hashicorp/random`, HCP Terraform backend, Helm (CloudNativePG operator + `Cluster` CR; Bitnami `redis`), cloud-init.

**Spec:** [`docs/superpowers/specs/2026-06-27-multicloud-terraform-migration-design.md`](../specs/2026-06-27-multicloud-terraform-migration-design.md)

---

## Verification gates (TDD analog for infra)

No Terraform unit-test framework in this repo. Each task's "test" is the standard infra gate:
- `terraform fmt -check` (formatting)
- `terraform init -backend=false && terraform validate` (HCL + type correctness; no cloud credentials needed)
- `helm lint` (chart correctness) after `helm dependency build`

Run the gate after writing code; expect PASS before committing. `terraform plan`/`apply` (credentials required) run in CI and in the final smoke test.

## File Structure

```
infra/
├── terraform/
│   ├── providers/
│   │   ├── shared/
│   │   │   └── cloudinit/            # NEW — shared cloud-init rendering (DRY)
│   │   │       ├── versions.tf       # required_providers: cloudinit
│   │   │       ├── variables.tf
│   │   │       ├── main.tf
│   │   │       └── outputs.tf
│   │   └── hetzner/                  # NEW — hcloud impl of the contract
│   │       ├── versions.tf           # required_providers: hcloud
│   │       ├── variables.tf          # contract INPUTS (worker_pool has labels/taints)
│   │       ├── main.tf               # ssh key, network, firewall, servers, LB
│   │       └── outputs.tf            # contract OUTPUTS
│   ├── modules/
│   │   └── dns/                      # REWRITE — Route53 → Cloudflare
│   │       ├── versions.tf
│   │       ├── variables.tf
│   │       ├── main.tf
│   │       └── outputs.tf
│   └── environments/
│       └── staging/
│           ├── versions.tf           # NEW — required_providers: random
│           ├── variables.tf
│           ├── main.tf               # REWRITE — compose module "cloud" + dns + random_password
│           ├── backend.tf            # REWRITE — S3/DynamoDB → HCP Terraform
│           └── terraform.tfvars.example
├── cloud-init/
│   ├── common.yaml                   # unchanged content, NOW applied via shared module
│   ├── k3s-server.yaml               # add pre-shared --token
│   └── k3s-agent.yaml                # add pre-shared token + node label/taint args
└── helm/
    ├── postgres/                     # NEW — CloudNativePG (operator + Cluster CR)
    │   ├── Chart.yaml
    │   ├── values.yaml
    │   └── templates/{cluster.yaml, NOTES.txt}
    └── redis/                        # NEW — Bitnami redis wrapper
        ├── Chart.yaml
        ├── values.yaml
        └── templates/NOTES.txt
```

DELETE (AWS-specific): `infra/terraform/providers/{aws.tf,versions.tf}`, `infra/terraform/modules/{vpc,k3s,node_pool,rds,elasticache}/`.

---

## Task 1: Cloudflare DNS module

Replaces Route53 `modules/dns/` with a cloud-agnostic Cloudflare module.

**Files:**
- Create: `infra/terraform/modules/dns/versions.tf`
- Modify: `infra/terraform/modules/dns/variables.tf` (rewrite)
- Modify: `infra/terraform/modules/dns/main.tf` (rewrite)
- Create: `infra/terraform/modules/dns/outputs.tf`

- [ ] **Step 1: Create `infra/terraform/modules/dns/versions.tf`**

```hcl
terraform {
  required_version = ">= 1.7.0"
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.40"
    }
  }
}
```

- [ ] **Step 2: Rewrite `infra/terraform/modules/dns/variables.tf`**

```hcl
variable "cloudflare_api_token" {
  type      = string
  sensitive = true
}

variable "zone_name" {
  type = string
}

variable "hostname" {
  type = string
}

# LB hostname (CNAME) or IP (A) from the provider module
variable "target" {
  type = string
}

variable "proxied" {
  type    = bool
  default = false
}
```

- [ ] **Step 3: Rewrite `infra/terraform/modules/dns/main.tf`**

```hcl
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
```

- [ ] **Step 4: Create `infra/terraform/modules/dns/outputs.tf`**

```hcl
output "record_name" {
  value = cloudflare_record.gateway.hostname
}

output "record_id" {
  value = cloudflare_record.gateway.id
}
```

- [ ] **Step 5: Verify**

Run:
```bash
terraform -chdir=infra/terraform/modules/dns init -backend=false
terraform -chdir=infra/terraform/modules/dns fmt -check
terraform -chdir=infra/terraform/modules/dns validate
```
Expected: `init` downloads `cloudflare/cloudflare`; `fmt` exit 0; `validate` → `Success! The configuration is valid.`

- [ ] **Step 6: Commit**

```bash
git add infra/terraform/modules/dns/
git commit -m "feat(infra): rewrite dns module to Cloudflare for cross-cloud portability"
```

---

## Task 2: cloud-init pre-shared token + node scheduling args

Fixes the `k3s_token` placeholder and adds node label/taint flag interpolation for workers.

**Files:**
- Modify: `infra/cloud-init/k3s-server.yaml`
- Modify: `infra/cloud-init/k3s-agent.yaml`

- [ ] **Step 1: Rewrite `infra/cloud-init/k3s-server.yaml`**

```yaml
#cloud-config
write_files:
  - path: /etc/rancher/k3s/config.yaml
    content: |
      tls-san:
        - "${server_private_ip}"
runcmd:
  - curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server --cluster-init --token ${k3s_token}" sh -
  - chmod 644 /etc/rancher/k3s/k3s.yaml
```

- [ ] **Step 2: Rewrite `infra/cloud-init/k3s-agent.yaml`**

```yaml
#cloud-config
runcmd:
  - curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="agent ${node_label_args} ${node_taint_args}" K3S_URL=https://${server_private_ip}:6443 K3S_TOKEN=${k3s_token} sh -
```

- [ ] **Step 3: Verify cloud-init syntax (if cloud-init CLI installed)**

Run:
```bash
cloud-init schema --config-file infra/cloud-init/k3s-server.yaml && \
cloud-init schema --config-file infra/cloud-init/k3s-agent.yaml
```
Expected: `Valid schema` for both. If `cloud-init` is absent, skip — the `${...}` vars are validated by `templatefile` in Task 4's `terraform validate`.

- [ ] **Step 4: Commit**

```bash
git add infra/cloud-init/k3s-server.yaml infra/cloud-init/k3s-agent.yaml
git commit -m "fix(cloud-init): pre-shared k3s token + node label/taint args"
```

---

## Task 3: Hetzner provider module — providers + contract inputs

**Files:**
- Create: `infra/terraform/providers/hetzner/versions.tf`
- Create: `infra/terraform/providers/hetzner/variables.tf`

- [ ] **Step 1: Create `infra/terraform/providers/hetzner/versions.tf`**

```hcl
terraform {
  required_version = ">= 1.7.0"
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}
```

- [ ] **Step 2: Create `infra/terraform/providers/hetzner/variables.tf`**

```hcl
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
```

- [ ] **Step 3: Verify**

Run:
```bash
terraform -chdir=infra/terraform/providers/hetzner init -backend=false
terraform -chdir=infra/terraform/providers/hetzner validate
```
Expected: `init` downloads `hcloud`; `validate` → `Success!`

- [ ] **Step 4: Commit**

```bash
git add infra/terraform/providers/hetzner/versions.tf infra/terraform/providers/hetzner/variables.tf
git commit -m "feat(infra): hetzner provider module scaffolding + contract inputs (labels/taints)"
```

---

## Task 4: Shared cloud-init module

DRYs cloud-init rendering so every provider reuses it. Applies `common.yaml` (fixing the latent "never applied" bug) + the role file via MIME multipart, and converts `labels`/`taints` to k3s flags.

**Files:**
- Create: `infra/terraform/providers/shared/cloudinit/versions.tf`
- Create: `infra/terraform/providers/shared/cloudinit/variables.tf`
- Create: `infra/terraform/providers/shared/cloudinit/main.tf`
- Create: `infra/terraform/providers/shared/cloudinit/outputs.tf`

- [ ] **Step 1: Create `infra/terraform/providers/shared/cloudinit/versions.tf`**

```hcl
terraform {
  required_version = ">= 1.7.0"
  required_providers {
    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = "~> 2.3"
    }
  }
}
```

- [ ] **Step 2: Create `infra/terraform/providers/shared/cloudinit/variables.tf`**

```hcl
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
```

- [ ] **Step 3: Create `infra/terraform/providers/shared/cloudinit/main.tf`**

```hcl
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
```

- [ ] **Step 4: Create `infra/terraform/providers/shared/cloudinit/outputs.tf`**

```hcl
output "rendered" {
  value = data.cloudinit_config.this.rendered
}
```

- [ ] **Step 5: Verify**

Run:
```bash
terraform -chdir=infra/terraform/providers/shared/cloudinit fmt -check
terraform -chdir=infra/terraform/providers/shared/cloudinit init -backend=false
terraform -chdir=infra/terraform/providers/shared/cloudinit validate
```
Expected: `fmt` exit 0; `init` downloads `cloudinit`; `validate` → `Success!`

- [ ] **Step 6: Commit**

```bash
git add infra/terraform/providers/shared/cloudinit/
git commit -m "feat(infra): shared cloud-init module (merged common+role, node scheduling args)"
```

---

## Task 5: Hetzner provider module — network, ssh key, firewall

**Files:**
- Create: `infra/terraform/providers/hetzner/main.tf`

- [ ] **Step 1: Create `infra/terraform/providers/hetzner/main.tf`**

```hcl
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
```

- [ ] **Step 2: Verify**

Run:
```bash
terraform -chdir=infra/terraform/providers/hetzner fmt -check
terraform -chdir=infra/terraform/providers/hetzner init -backend=false
terraform -chdir=infra/terraform/providers/hetzner validate
```
Expected: `fmt` exit 0; `validate` → `Success!`

- [ ] **Step 3: Commit**

```bash
git add infra/terraform/providers/hetzner/main.tf
git commit -m "feat(infra): hetzner network, ssh key, and firewall"
```

---

## Task 6: Hetzner provider module — control-plane + worker nodes (via shared module)

**Files:**
- Modify: `infra/terraform/providers/hetzner/main.tf` (append)

- [ ] **Step 1: Append the server resources to `infra/terraform/providers/hetzner/main.tf`**

```hcl
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
  depends_on = [hcloud_network_subnet.this]

  network {
    network_id = hcloud_network.this.id
    ip         = cidrhost(var.network_cidr, local.worker_ip_base + count.index)
  }
}
```

> Note: `worker_cloudinit` renders once for the whole pool (all workers share the pool's labels/taints). For `control_plane.count > 1` (future HA) the join semantics change; staging uses `count = 1`.

- [ ] **Step 2: Verify**

Run:
```bash
terraform -chdir=infra/terraform/providers/hetzner fmt -check
terraform -chdir=infra/terraform/providers/hetzner init -backend=false
terraform -chdir=infra/terraform/providers/hetzner validate
```
Expected: `fmt` exit 0; `validate` → `Success!`

- [ ] **Step 3: Commit**

```bash
git add infra/terraform/providers/hetzner/main.tf
git commit -m "feat(infra): hetzner k3s control-plane + worker nodes via shared cloud-init"
```

---

## Task 7: Hetzner provider module — load balancer + contract outputs

**Files:**
- Modify: `infra/terraform/providers/hetzner/main.tf` (append LB)
- Create: `infra/terraform/providers/hetzner/outputs.tf`

- [ ] **Step 1: Append the load balancer to `infra/terraform/providers/hetzner/main.tf`**

```hcl
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
```

- [ ] **Step 2: Create `infra/terraform/providers/hetzner/outputs.tf`**

```hcl
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
```

- [ ] **Step 3: Verify**

Run:
```bash
terraform -chdir=infra/terraform/providers/hetzner fmt -check
terraform -chdir=infra/terraform/providers/hetzner init -backend=false
terraform -chdir=infra/terraform/providers/hetzner validate
```
Expected: `fmt` exit 0; `validate` → `Success!`

- [ ] **Step 4: Commit**

```bash
git add infra/terraform/providers/hetzner/main.tf infra/terraform/providers/hetzner/outputs.tf
git commit -m "feat(infra): hetzner load balancer + provider contract outputs"
```

---

## Task 8: Staging environment — variables + versions

**Files:**
- Create: `infra/terraform/environments/staging/versions.tf`
- Modify: `infra/terraform/environments/staging/variables.tf` (rewrite)

- [ ] **Step 1: Create `infra/terraform/environments/staging/versions.tf`**

```hcl
terraform {
  required_version = ">= 1.7.0"
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}
```

- [ ] **Step 2: Rewrite `infra/terraform/environments/staging/variables.tf`**

```hcl
# --- Cloud-agnostic ---
variable "ssh_pub_key" {
  type = string
}

variable "dns_zone" {
  type = string
}

# --- Cloud credentials (the only lines that change when switching cloud) ---
variable "hcloud_token" {
  type      = string
  sensitive = true
}

variable "cloudflare_api_token" {
  type      = string
  sensitive = true
}
```

- [ ] **Step 3: Verify**

Run:
```bash
terraform -chdir=infra/terraform/environments/staging fmt -check
```
Expected: exit 0. (Full `validate` runs in Task 9 once `main.tf`/`backend.tf` exist.)

- [ ] **Step 4: Commit**

```bash
git add infra/terraform/environments/staging/versions.tf infra/terraform/environments/staging/variables.tf
git commit -m "feat(infra): staging env variables + required providers"
```

---

## Task 9: Staging environment — compose + HCP backend

**Files:**
- Modify: `infra/terraform/environments/staging/main.tf` (rewrite)
- Modify: `infra/terraform/environments/staging/backend.tf` (rewrite)
- Modify: `infra/terraform/environments/staging/terraform.tfvars.example` (rewrite)

- [ ] **Step 1: Rewrite `infra/terraform/environments/staging/main.tf`**

```hcl
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
```

- [ ] **Step 2: Rewrite `infra/terraform/environments/staging/backend.tf`**

Replace S3+DynamoDB with HCP Terraform. Set `<ORG>` to your HCP Terraform organization.

```hcl
terraform {
  cloud {
    organization = "<ORG>"

    workspaces {
      name = "spatial-staging"
    }
  }
}
```

- [ ] **Step 3: Rewrite `infra/terraform/environments/staging/terraform.tfvars.example`**

```
# ssh_pub_key          = "ssh-ed25519 AAA..."
# dns_zone             = "spatial.example.com"
# hcloud_token         = "<Hetzner Cloud API token>"
# cloudflare_api_token = "<Cloudflare API token>"
```

- [ ] **Step 4: Verify**

Run (backend disabled so no HCP login needed):
```bash
terraform -chdir=infra/terraform/environments/staging fmt -check
terraform -chdir=infra/terraform/environments/staging init -backend=false
terraform -chdir=infra/terraform/environments/staging validate
```
Expected: `fmt` exit 0; `init` resolves `hcloud`, `cloudinit`, `cloudflare`, `random` via child modules; `validate` → `Success!`

- [ ] **Step 5: Commit**

```bash
git add infra/terraform/environments/staging/main.tf infra/terraform/environments/staging/backend.tf infra/terraform/environments/staging/terraform.tfvars.example
git commit -m "feat(infra): compose staging on hetzner (labeled workers) + HCP Terraform backend"
```

---

## Task 10: Delete AWS-specific modules + providers

**Files:**
- Delete: `infra/terraform/providers/aws.tf`, `infra/terraform/providers/versions.tf`
- Delete: `infra/terraform/modules/{vpc,k3s,node_pool,rds,elasticache}/` (all)

- [ ] **Step 1: Delete the files**

```bash
git rm infra/terraform/providers/aws.tf infra/terraform/providers/versions.tf
git rm -r infra/terraform/modules/vpc infra/terraform/modules/k3s \
          infra/terraform/modules/node_pool infra/terraform/modules/rds \
          infra/terraform/modules/elasticache
```

- [ ] **Step 2: Verify nothing references the deleted modules**

Run:
```bash
rg -n "modules/vpc|modules/k3s|modules/node_pool|modules/rds|modules/elasticache|providers/aws|hashicorp/aws" infra/ || true
```
Expected: no matches. If matches remain, fix them before continuing.

- [ ] **Step 3: Re-validate staging**

Run:
```bash
terraform -chdir=infra/terraform/environments/staging init -backend=false
terraform -chdir=infra/terraform/environments/staging validate
```
Expected: `Success!`

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore(infra): remove AWS-specific terraform modules and providers"
```

---

## Task 11: PostgreSQL via CloudNativePG

Operator-based, K8s-native Postgres (failover, PITR, rolling upgrades). Redis stays Bitnami (Task 12).

**Files:**
- Create: `infra/helm/postgres/Chart.yaml`
- Create: `infra/helm/postgres/values.yaml`
- Create: `infra/helm/postgres/templates/cluster.yaml`
- Create: `infra/helm/postgres/templates/NOTES.txt`

- [ ] **Step 1: Create `infra/helm/postgres/Chart.yaml`**

```yaml
apiVersion: v2
name: postgres
description: PostgreSQL for spatial-server via CloudNativePG (cloud-agnostic, K8s-native)
type: application
version: 0.1.0
appVersion: "16"
dependencies:
  - name: cloudnative-pg
    version: "0.28.x"
    repository: "https://cloudnative-pg.io/charts"
    condition: operator.enabled
```

- [ ] **Step 2: Create `infra/helm/postgres/values.yaml`**

```yaml
# Install the CNPG operator as part of this release. Set false if the operator
# is already installed cluster-wide.
operator:
  enabled: true

image:
  tag: "16"

database: spatial
owner: spatial

instances: 1            # bump to 3 for HA in production

storage:
  # Cloud-agnostic knob, set per environment. "" uses the cluster default
  # StorageClass. Use a cloud CSI class (e.g. "hcloud") in production;
  # NEVER rely on local-path for production DBs.
  storageClass: ""
  size: 20Gi

# PITR backups to S3-compatible object storage (provider-specific). Enable when
# a bucket + credentials secret exist.
backups:
  enabled: false
  destinationPath: ""    # e.g. s3://spatial-pg-backups
  endpointURL: ""        # e.g. https://fsn1.your-objectstorage.com
  secretName: cnpg-backup-creds

resources:
  requests: { cpu: 250m, memory: 512Mi }
  limits:   { cpu: "1", memory: 1Gi }
```

- [ ] **Step 3: Create `infra/helm/postgres/templates/cluster.yaml`**

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: spatial-postgres
  labels:
    app.kubernetes.io/name: postgres
    app.kubernetes.io/part-of: spatial-server
spec:
  imageName: ghcr.io/cloudnative-pg/postgresql:{{ .Values.image.tag }}
  instances: {{ .Values.instances | int }}
  bootstrap:
    initdb:
      database: {{ .Values.database | quote }}
      owner: {{ .Values.owner | quote }}
  storage:
    storageClass: {{ .Values.storage.storageClass | quote }}
    size: {{ .Values.storage.size }}
  {{- if .Values.backups.enabled }}
  backup:
    barmanObjectStore:
      destinationPath: {{ .Values.backups.destinationPath | quote }}
      {{- with .Values.backups.endpointURL }}
      endpointURL: {{ . | quote }}
      {{- end }}
      s3Credentials:
        accessKeyId:
          name: {{ .Values.backups.secretName }}
          key: ACCESS_KEY_ID
        secretAccessKey:
          name: {{ .Values.backups.secretName }}
          key: ACCESS_SECRET_KEY
      wal:
        retention: 7d
  retentionPolicy: "14d"
  {{- end }}
  resources:
    {{- toYaml .Values.resources | nindent 4 }}
```

- [ ] **Step 4: Create `infra/helm/postgres/templates/NOTES.txt`**

```
PostgreSQL for spatial-server via CloudNativePG.

Operator: with operator.enabled=true, the CloudNativePG operator is installed by this release.
Credentials: CNPG auto-creates Secrets "spatial-postgres-app" and "spatial-postgres-superuser".
Storage: set values.storage.storageClass to your cloud CSI class (e.g. "hcloud"); "" = default.
Backups: set backups.enabled + an S3-compatible target to enable PITR.

Future: if a cloud offers managed Postgres, replace this chart with a provider endpoint
and keep the same Secret names consumed by services.
```

- [ ] **Step 5: Verify**

Run:
```bash
helm dependency build infra/helm/postgres
helm lint infra/helm/postgres
```
Expected: `dependency build` downloads the `cloudnative-pg` operator chart; `helm lint` → `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 6: Commit**

```bash
git add infra/helm/postgres/
git commit -m "feat(helm): PostgreSQL via CloudNativePG (operator + Cluster CR) for cloud-agnostic DB"
```

---

## Task 12: Redis Helm chart (Bitnami)

**Files:**
- Create: `infra/helm/redis/Chart.yaml`
- Create: `infra/helm/redis/values.yaml`
- Create: `infra/helm/redis/templates/NOTES.txt`

- [ ] **Step 1: Create `infra/helm/redis/Chart.yaml`**

```yaml
apiVersion: v2
name: redis
description: Redis for spatial-server (self-managed, cloud-agnostic)
type: application
version: 0.1.0
appVersion: "7"
dependencies:
  - name: redis
    version: "20.6.x"
    repository: "https://charts.bitnami.com/bitnami"
    condition: redis.enabled
```

- [ ] **Step 2: Create `infra/helm/redis/values.yaml`**

```yaml
managed:
  redis: false

redis:
  enabled: true
  architecture: replication
  auth:
    password: "change-me"   # override in prod
  master:
    persistence:
      enabled: true
      size: 8Gi
  replica:
    replicaCount: 1
    persistence:
      enabled: true
      size: 8Gi
```

- [ ] **Step 3: Create `infra/helm/redis/templates/NOTES.txt`**

```
Redis for spatial-server (Bitnami, replication mode, cloud-agnostic).
When managed.redis = true, set redis.enabled = false and supply a managed endpoint.
```

- [ ] **Step 4: Verify**

Run:
```bash
helm dependency build infra/helm/redis
helm lint infra/helm/redis
```
Expected: `Successfully generated chart`; `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 5: Commit**

```bash
git add infra/helm/redis/
git commit -m "feat(helm): self-managed redis chart (Bitnami) for cloud-agnostic cache"
```

---

## Task 13: CI workflow — Hetzner/Cloudflare/HCP secrets

**Files:**
- Modify: `.github/workflows/terraform-plan.yml`

- [ ] **Step 1: Read the current workflow**

Run:
```bash
cat .github/workflows/terraform-plan.yml
```
Note the existing job structure so you only change auth env vars + the backend login.

- [ ] **Step 2: Replace AWS auth with HCP Terraform + Hetzner/Cloudflare**

In the job that runs `terraform init`/`plan`, replace AWS credential env vars with HCP login + the new provider env vars:

```yaml
      - name: Configure Terraform Cloud
        run: |
          cat > ~/.terraformrc <<EOF
          credentials "app.terraform.io" {
            token = "${{ secrets.TF_API_TOKEN }}"
          }
          EOF

      - name: Terraform Init
        env:
          HCLOUD_TOKEN:               ${{ secrets.HCLOUD_TOKEN }}
          TF_VAR_hcloud_token:        ${{ secrets.HCLOUD_TOKEN }}
          TF_VAR_cloudflare_api_token: ${{ secrets.CLOUDFLARE_API_TOKEN }}
        run: terraform -chdir=infra/terraform/environments/staging init

      - name: Terraform Plan
        env:
          HCLOUD_TOKEN:               ${{ secrets.HCLOUD_TOKEN }}
          TF_VAR_hcloud_token:        ${{ secrets.HCLOUD_TOKEN }}
          TF_VAR_cloudflare_api_token: ${{ secrets.CLOUDFLARE_API_TOKEN }}
        run: terraform -chdir=infra/terraform/environments/staging plan -no-color
```

Add GitHub secrets `TF_API_TOKEN`, `HCLOUD_TOKEN`, `CLOUDFLARE_API_TOKEN`. Remove any `AWS_*` references.

- [ ] **Step 3: Verify YAML**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/terraform-plan.yml'))" && echo OK
```
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/terraform-plan.yml
git commit -m "ci(terraform): switch plan workflow to HCP Terraform + Hetzner/Cloudflare secrets"
```

---

## Task 14: Docs — amend ADR-014, add ADR-024, update Phase 5 spec

**Files:**
- Modify: `docs/adr/014-infrastructure-platform.md`
- Create: `docs/adr/024-multicloud-provider-abstraction.md`
- Modify: `docs/adr/README.md`
- Modify: `docs/superpowers/specs/phase-5-infra-as-code.md`

- [ ] **Step 1: Amend ADR-014 — append a "Cloud Selection" note under Decision**

Append to `docs/adr/014-infrastructure-platform.md`:

```markdown
### Cloud Selection

The platform remains cloud-agnostic by design; only the IaaS Terraform layer is cloud-specific.
Concrete providers implement a shared provider contract (see
[ADR-024](024-multicloud-provider-abstraction.md)):

- **Hetzner Cloud** — staging/test (first implemented provider).
- **Sakura Internet / AWS** — production candidates (future provider modules, same contract).

Switching an environment's cloud changes only `module "cloud" { source = ... }` and the
cloud-credential variable in that environment.
```

- [ ] **Step 2: Create `docs/adr/024-multicloud-provider-abstraction.md`**

```markdown
# ADR 024: Multi-cloud Provider Abstraction

## Status
Accepted

## Context
[ADR-014](014-infrastructure-platform.md) mandates cloud agnosticism, but the initial Terraform
was AWS-specific (VPC, RDS, ElastiCache, Route53, S3/DynamoDB backend). Staging runs on Hetzner;
production will run on a different cloud (Sakura Internet or AWS). We must add clouds without
rewriting the platform.

## Problem
Cloud-specific Terraform couples the platform to one provider and makes switching clouds expensive.

## Decision
Isolate the cloud-specific IaaS layer behind a **provider contract**. Each cloud lives in
`infra/terraform/providers/<cloud>/` and consumes shared cloud-init rendering from
`providers/shared/cloudinit/`. Every provider exposes identical inputs/outputs.

**Inputs:** `cluster_name`, `ssh_pub_key`, `k3s_token`, `control_plane { server_type, count }`,
`worker_pool { server_type, count, labels, taints }`, `network_cidr`, `allowed_ssh_cidrs`, `location`.

**Outputs:** `control_plane_private_ips`, `control_plane_public_ips`, `worker_private_ips`,
`load_balancer_endpoint` (IP or hostname, per cloud), `load_balancer_ip`, `network_cidr`.

`worker_pool.labels`/`.taints` become k3s `--node-label`/`--node-taint` so workload scheduling
(node roles, dedicated pools) is supported without changing the contract later.

### Data services
- **PostgreSQL:** CloudNativePG operator (native failover, PITR, rolling upgrades), cloud-agnostic.
- **Redis:** Bitnami chart.
- Neither ties to a managed offering; PVCs use a per-cloud CSI StorageClass (`local-path` dev only).

Everything above the contract — cloud-init, Helm, Cloudflare DNS, HCP Terraform state — is shared.

Switching an environment's cloud changes only `module "cloud"` source + the credential variable.

## Alternatives
1. Single module with a `cloud` variable and per-cloud conditionals — rejected: conditional sprawl.
2. Infra/Platform split (two Terraform layers/states) — deferred as future evolution.

## Tradeoffs
- Adding a cloud requires a provider module that matches the contract exactly.
- Cloud-specific knobs (`server_type`) are exposed per provider; only the contract shape is portable.

## Consequences
- Databases are self-managed in K3s (CNPG/Bitnami) because Hetzner has no managed Postgres/Redis.
- Worker pools are fixed-count; Cluster Autoscaler can be layered on later.
- State in HCP Terraform; DNS in Cloudflare — both cloud-independent.

## Future Considerations
- Sakura Internet and AWS provider modules (same contract).
- HA control-plane (outputs are already list-shaped).
- Dedicated node pools (`workload=gateway`, `workload=database`).
- Cluster Autoscaler; External Secrets Operator; Velero; monitoring stack (ADR-019).

## References
- [ADR-014 Infrastructure Platform](014-infrastructure-platform.md)
- [ADR-019 Observability](019-observability.md)
- [Multi-cloud migration design](../superpowers/specs/2026-06-27-multicloud-terraform-migration-design.md)
```

- [ ] **Step 3: Add ADR-024 to `docs/adr/README.md`**

Read the file, match the existing table style, and append a row for ADR-024.

- [ ] **Step 4: Update the Phase 5 spec — cloud references**

In `docs/superpowers/specs/phase-5-infra-as-code.md`, refresh the Terraform-modules section:
- `providers/aws.tf (AWS reference impl)` → providers are cloud-specific under `providers/<cloud>/`
  (Hetzner staging; link ADR-024).
- RDS/ElastiCache → CloudNativePG Postgres + Bitnami Redis in K3s (ADR-024).
- Route53 → Cloudflare; S3/DynamoDB backend → HCP Terraform.

Add `> **Last Updated:** 2026-06-27` if not present.

- [ ] **Step 5: Commit**

```bash
git add docs/adr/014-infrastructure-platform.md docs/adr/024-multicloud-provider-abstraction.md docs/adr/README.md docs/superpowers/specs/phase-5-infra-as-code.md
git commit -m "docs(adr): multi-cloud provider abstraction (ADR-024) + amend ADR-014"
```

---

## Final smoke test (post-apply, manual, needs credentials)

```bash
terraform -chdir=infra/terraform/environments/staging apply

# Cluster bring-up: install Hetzner cloud-controller-manager + CSI driver so the
# cluster gets a real `hcloud` StorageClass (required for CNPG PVCs in production).
helm repo add hetzner https://charts.hetzner.cloud
helm install hcloud-ccm hetzner/hcloud-cloud-controller-manager -n kube-system   # wire HCLOUD_TOKEN per chart README
helm install hcloud-csi hetzner/hcloud-csi-driver               -n kube-system   # wire HCLOUD_TOKEN per chart README
kubectl get storageclass                  # expect an `hcloud` class

kubectl get nodes                         # cp + workers Ready
kubectl get nodes --show-labels           # workers carry workload=game
helm install postgres infra/helm/postgres -n spatial-server --set storage.storageClass=hcloud
helm install redis    infra/helm/redis    -n spatial-server
kubectl get pods -n spatial-server        # gateway/room/game/postgres/redis Ready
dig +short gateway.<dns_zone>             # resolves to the Hetzner LB IP
```

## Cross-cloud switch (later, for reference)

To move production to Sakura Internet (once `providers/sakura/` exists):
1. Set `module "cloud" { source = "../../providers/sakura" ... }` in `environments/production/`.
2. Swap `hcloud_token` for the Sakura equivalent + provider credentials.
3. Set the Helm `storageClass` value to the Sakura CSI class.
4. Everything else (cloud-init, Helm, DNS, state) is unchanged.

---

## Plan Self-Review

**Spec coverage** (each spec section → task):
- Cloudflare DNS module → Task 1
- cloud-init token + node args → Task 2 (+ applied via Task 4 `cloudinit_config`)
- Provider contract inputs (incl. `worker_pool.labels/taints`) → Task 3
- Shared cloud-init module → Task 4
- Hetzner network/firewall → Task 5; servers → Task 6; LB + outputs → Task 7
- Node roles/scheduling → Task 3 (contract) + Task 6 (labels flow) + Task 9 (example)
- Staging composition + HCP backend → Tasks 8, 9
- AWS deletion → Task 10
- CloudNativePG PostgreSQL → Task 11; Bitnami Redis → Task 12
- StorageClass strategy → spec section + smoke test CSI bring-up
- Service exposure → spec section (Traefik-only in LB, Task 7) + NetworkPolicies unchanged
- Secrets / monitoring / backup / upgrade → spec roadmap sections (documentation only)
- CI → Task 13; Docs → Task 14
- No gaps.

**Placeholder scan:** none — every step has complete code and exact commands. `<ORG>` is an org-specific config value to substitute (documented).

**Type/name consistency:** `worker_pool` shape (`server_type`, `count`, `labels`, `taints`) is identical in Task 3 (variable), Task 6 (`var.worker_pool.labels`/`.taints`), and Task 9 (example). Contract outputs in Task 7 match Task 9's `module.cloud.load_balancer_endpoint` and ADR-024. Shared-module vars (`ssh_pub_key`, `server_private_ip`, `k3s_token`, `node_labels`, `node_taints`) match Task 2's template variables (`server_private_ip`, `k3s_token`, `node_label_args`, `node_taint_args`). `random_password.k3s_token.result` flows from Task 9 → `module.cloud` → shared module → both server + agent cloud-init.
