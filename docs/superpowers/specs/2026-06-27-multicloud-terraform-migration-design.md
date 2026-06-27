# Multi-cloud Terraform Migration (AWS → Provider Abstraction, Hetzner first)

> **Last Updated:** 2026-06-27
> **Status:** Draft (rev 2 — review hardening applied)
> **Supersedes (in part):** The AWS reference implementation described in [Phase 5 — Infra-as-Code](./phase-5-infra-as-code.md)

## Purpose

Replace the current AWS-specific Terraform with a **cloud-agnostic, multi-provider** architecture. Hetzner Cloud is the first concrete provider (used for **test/staging**). Production will run on a different cloud (Sakura Internet or AWS) later. Adding a cloud must require only a new `providers/<cloud>/` module plus a one-line `source` change in the environment — no changes to cloud-init, Helm charts, or K3s manifests.

This spec defines the provider contract, the Hetzner reference implementation, shared cloud-agnostic logic, the supporting modules (Cloudflare DNS, HCP Terraform state, CloudNativePG PostgreSQL, Bitnami Redis), and the operational policies/roadmaps that make the platform production-ready and portable.

## Context

The platform is **K3s on VMs + cloud-init + Helm**, per [ADR-014 Infrastructure Platform](../../adr/014-infrastructure-platform.md) and [ADR-008 Deployment](../../adr/008-deployment.md). ADR-014 mandates cloud agnosticism, but the current Terraform is AWS-coupled (`terraform-aws-modules/vpc`, `aws_instance`, `aws_autoscaling_group`, RDS, ElastiCache, Route53, S3/DynamoDB backend).

Because K3s + cloud-init + Helm are inherently cloud-agnostic, **only the IaaS Terraform layer is cloud-specific**. The migration isolates that layer behind a provider contract so the rest of the platform is reused verbatim across clouds.

### Current AWS state (to be replaced)

| AWS resource | Module |
|---|---|
| VPC + public/private subnets + NAT | `modules/vpc` (`terraform-aws-modules/vpc` v5.8.1) |
| K3s control-plane EC2 (`t3.medium`, Ubuntu 22.04) | `modules/k3s` |
| K3s agent ASG (`t3.large`, desired 2, 1–6) | `modules/node_pool` |
| RDS PostgreSQL 16 (`db.t4g.medium`, managed) | `modules/rds` |
| ElastiCache Redis 7 (`cache.t4g.small`, 2-node HA) | `modules/elasticache` |
| Route53 A record → LB alias | `modules/dns` |
| S3 + DynamoDB state/lock | `environments/staging/backend.tf` |

Known issues in the current code: `k3s_token = "WIRE_FROM_K3S_OUTPUT"` (placeholder — agents cannot join); `cloud-init/common.yaml` (user hardening, fail2ban, ulimits) is never applied. Both are fixed as part of this migration.

## Goals

- **One contract, N clouds.** Every provider implements the same inputs/outputs.
- **Cloud-swap in one line.** Switching an environment's cloud = change `module "cloud"` `source` (plus the cloud-credential variable).
- **Shared bootstrap, DRY.** Cloud-init rendering lives in `providers/shared/`; provider modules only declare cloud resources.
- **Portable state.** HCP Terraform backend, decoupled from any cloud.
- **Portable DNS.** Cloudflare DNS so records survive a cloud change.
- **Portable, production-grade data services.** PostgreSQL via CloudNativePG (K8s-native HA/backup), Redis via Bitnami — neither tied to a managed offering.
- **Scheduling-ready.** Worker pools carry k3s node labels/taints so workloads can be isolated without changing the contract later.
- **No data migration.** Staging is greenfield; production cloud does not exist yet.

## Non-Goals

- Migrating live data (none exists in staging).
- Implementing the Sakura/AWS provider modules now (only the contract + Hetzner). Those are future work under the same contract.
- Cluster Autoscaler integration now (worker pool is fixed-count; the contract allows an autoscaler layer later).
- Production HA control-plane now (single control-plane node; outputs are list-shaped and HA-ready).
- TLS cert issuance (remains Phase 6, per [Phase 5 spec](./phase-5-infra-as-code.md)).
- Deploying the monitoring stack, Velero, or External Secrets now (roadmap documentation only).
- CSI/cloud-controller installation via Terraform (provider-specific cluster bring-up, done via Helm post-apply, per [ADR-014](../../adr/014-infrastructure-platform.md)).

## Architecture

```
                ┌─────────────────────────────────────────────────┐
  Cloud-agnostic│  Helm charts (gateway/room-service/game-server,  │  ← identical
  (shared)      │   postgres=CloudNativePG, redis=Bitnami)         │     on every
                │  K3s manifests (infra/k3s/*)                      │     cloud
                │  providers/shared/  (cloud-init rendering)        │
                │  Cloudflare DNS module (modules/dns)              │  ← provider-agnostic
                │  HCP Terraform state backend                      │
                └───────────────────────┬─────────────────────────┘
                                        │ Provider Contract (inputs/outputs)
                ┌───────────────────────▼─────────────────────────┐
  Cloud-specific│  providers/hetzner/   ← staging (implemented)    │
  (IaaS only)   │  providers/sakura/    ← production (future)      │
                │  providers/aws/       ← production (future)      │
                └─────────────────────────────────────────────────┘
                Secrets: Kubernetes Secret (now) → External Secrets (future)
                StorageClass: per-cloud CSI (prod) / local-path (dev)
```

```
                           Internet
                              │
                    ┌─────────▼──────────┐
                    │ Cloudflare DNS     │ ◄── modules/dns (A/CNAME → LB)
                    └─────────┬──────────┘
                              │ WSS :443 (cert in Phase 6)
            ┌─────────────────▼───────────────────┐
            │ Hetzner Cloud Network (10.0.0.0/16) │
            │   ┌──────────────────────────────┐  │
            │   │ Load Balancer (hcloud LB)    │──┼─► Traefik ONLY (:80/:443)
            │   └──────────────┬───────────────┘  │   (no other service public)
            │   ┌──────────────▼───────────────┐  │
            │   │ K3s control-plane (1 server) │  │  node-role: control-plane (auto)
            │   └──────────────┬───────────────┘  │
            │            token join (6443, priv) │
            │   ┌──────────────▼───────────────┐  │
            │   │ K3s workers (N servers)      │  │  labels: workload=game (+ taints)
            │   └──────────────────────────────┘  │
            └─────────────────────────────────────┘
                              │ (in-cluster, ClusterIP only)
            ┌─────────────────▼───────────────────┐
            │ Helm: gateway(public) / room-service│
            │       / game-server / postgres(CNPG)│
            │       / redis(Bitnami)              │
            └─────────────────────────────────────┘
```

## Provider Contract

Every `providers/<cloud>/` module MUST expose exactly this interface. The staging environment depends only on these names; a future Sakura/AWS module implementing the same names drops in without other changes.

### Inputs

| Variable | Type | Default | Notes |
|---|---|---|---|
| `cluster_name` | string | — | e.g. `spatial-staging` |
| `ssh_pub_key` | string | — | cloud-agnostic ed25519/rsa public key |
| `k3s_token` | string (sensitive) | — | pre-shared; produced by `random_password` in the environment |
| `control_plane` | object `{ server_type = string, count = number }` | `{ "cpx21", 1 }` | list-shapes outputs (HA-ready) |
| `worker_pool` | object (below) | — | carries scheduling metadata |
| `network_cidr` | string | `10.0.0.0/16` | private network range |
| `allowed_ssh_cidrs` | list(string) | `[]` | empty = no public SSH |
| `location` | string | cloud default | e.g. `nbg1` (Hetzner) |

`worker_pool` (scheduling-ready):

```hcl
worker_pool = {
  server_type = string        # cloud-specific (e.g. cpx31)
  count       = number
  labels      = map(string)   # k3s node labels, e.g. { workload = "game" }
  taints      = map(string)   # k3s node taints: key -> "value:Effect"
}
# defaults: labels = {}, taints = {}
```

### Outputs (contract — must be identical across providers)

| Output | Type | Consumed by |
|---|---|---|
| `control_plane_private_ips` | list(string) | agent cloud-init (join target) |
| `control_plane_public_ips` | list(string) | kubeconfig / operator access |
| `worker_private_ips` | list(string) | observability/inventory |
| `load_balancer_endpoint` | string | Cloudflare DNS target (IP→A / hostname→CNAME) |
| `load_balancer_ip` | string | smoke tests |
| `network_cidr` | string | firewall rule references |

### Node Roles & Scheduling

| Node | Role / labels | How |
|---|---|---|
| Control-plane | `node-role.kubernetes.io/control-plane` | automatic (k3s) |
| Workers | `workload=game` (example), optional taints | contract `worker_pool.labels` / `.taints` → k3s `--node-label` / `--node-taint` in cloud-init |
| Future dedicated pools | `workload=gateway`, `workload=database` | additional pool definitions (future) |

Workloads land via `nodeSelector`/`affinity`/`tolerations`. The contract carries labels/taints now so adding dedicated pools later changes data, not interface.

## Shared Cloud-init Module (`providers/shared/cloudinit/`)

Per the review's DRY requirement, cloud-init rendering is **shared**, not duplicated per provider. A single module renders the merged (MIME multipart) cloud-init for a node given `role`, `ssh_pub_key`, `server_private_ip`, `k3s_token`, `node_labels`, `node_taints`, and outputs `rendered`. It:

1. applies `infra/cloud-init/common.yaml` (user, fail2ban, ulimits per [ADR-017](../../adr/017-capacity-planning.md)) — fixing the latent "never applied" bug;
2. applies `k3s-server.yaml` or `k3s-agent.yaml` (merged via `list(append)+dict(recurse_list,no_replace)`);
3. converts `labels`/`taints` maps to `--node-label`/`--node-taint` flag strings for agents.

Each provider module calls `module "<role>_cloudinit" { source = "../shared/cloudinit" ... }` and passes `user_data = module.*.rendered` to its VM resource. Provider modules contain **only** cloud resources (network, firewall, servers, LB) + the contract outputs.

## Hetzner Reference Implementation (`providers/hetzner/`)

Provider: `hetznercloud/hcloud`. Resources: `hcloud_ssh_key`, `hcloud_network` + `hcloud_network_subnet`, `hcloud_firewall`, `hcloud_server` (control-plane + workers, `user_data` from the shared module), `hcloud_load_balancer` (+ targets/services). cloud-init rendering is delegated to `providers/shared/cloudinit`.

### AWS → Hetzner mapping

| AWS | Hetzner (hcloud) | Difference |
|---|---|---|
| VPC + subnets + NAT | `hcloud_network` + subnet | Servers carry public + private NIC; no NAT needed |
| EC2 (control-plane) | `hcloud_server` | — |
| ASG (agents) | `hcloud_server` × `count` | Fixed pool (no native ASG) |
| Security Group | `hcloud_firewall` | — |
| ALB/NLB | `hcloud_load_balancer` | Managed |
| Route53 | Cloudflare (`cloudflare_record`) | Provider-agnostic |
| RDS / ElastiCache | dropped → CloudNativePG / Bitnami (in K3s) | Self-managed, portable |
| S3 + DynamoDB | HCP Terraform | Decoupled state |

## cloud-init + K3s Token Fix

The placeholder `k3s_token = "WIRE_FROM_K3S_OUTPUT"` is replaced by a **pre-shared token** (`random_password.k3s_token`) passed into both server and agent cloud-init — deterministic, cloud-agnostic, no boot chicken-and-egg.

- `infra/cloud-init/k3s-server.yaml`: `INSTALL_K3S_EXEC="server --cluster-init --token ${k3s_token}"` + `tls-san ${server_private_ip}`.
- `infra/cloud-init/k3s-agent.yaml`: `INSTALL_K3S_EXEC="agent ${node_label_args} ${node_taint_args}" K3S_URL=https://${server_private_ip}:6443 K3S_TOKEN=${k3s_token}`.
- `infra/cloud-init/common.yaml`: content unchanged; now applied via the shared module's MIME merge.

Agents join via the control-plane **private IP** (port 6443), not the load balancer.

## Cloudflare DNS Module (`modules/dns/`)

Rewritten from Route53 to the `cloudflare` provider so DNS is independent of the compute cloud. Inputs: `zone_name`, `hostname`, `target` (LB hostname/IP), `proxied`. Creates an `A` (if IP) or `CNAME`. Identical on every cloud. `CLOUDFLARE_API_TOKEN` supplied at apply time; never committed.

## HCP Terraform State Backend

`environments/staging/backend.tf` switches from S3+DynamoDB to HCP Terraform (`cloud { organization; workspaces { name } }`). `TF_API_TOKEN` authenticates. State + locking are fully decoupled from any cloud.

## Self-Managed Databases

RDS/ElastiCache are dropped. Data services run inside K3s — portable and identical across clouds.

### PostgreSQL — CloudNativePG (operator-based, K8s-native)

Default PostgreSQL implementation is **CloudNativePG** (chosen over Bitnami for native failover, PITR backup, rolling upgrades, and operational maturity). Delivered by `infra/helm/postgres/`:

- Installs the CNPG operator (Helm chart `cloudnative-pg` from `https://cloudnative-pg.io/charts`) as a dependency.
- Templates a `Cluster` CR (`postgresql.cnpg.io/v1`): `instances` (1 staging / 3 prod HA), `initdb` bootstrap (`database=spatial`, `owner=spatial` — CNPG auto-manages credentials in `<cluster>-app`/`<cluster>-superuser` Secrets), `storage.storageClass` + `size`, optional `backup.barmanObjectStore` (PITR to provider object storage).

`storageClass` is a cloud-agnostic knob set per environment (see StorageClass Strategy). The chart is cloud-agnostic; only the `storageClass` value and the backup object-store target change per cloud.

### Redis — Bitnami (unchanged)

`infra/helm/redis/` wraps the Bitnami `redis` chart (replication mode + persistence).

Both charts keep a `managed` note: if a cloud later offers a managed equivalent and `managed` is enabled, the chart is skipped and a provider module supplies the endpoint — preserving a future managed path without coupling now.

## StorageClass Strategy

Persistent storage is **provider-specific at the cluster level, cloud-agnostic at the chart level**. Helm values expose `storageClass`; the actual class is set per environment.

| Environment | StorageClass | Source |
|---|---|---|
| Dev / local K3s | `local-path` | K3s built-in (ephemeral; **never for production DBs**) |
| Hetzner (prod/staging) | `hcloud` | Hetzner CSI driver (cluster bring-up, Helm) |
| AWS (future prod) | `gp3` | AWS EBS CSI driver |
| Sakura (future prod) | Sakura Block Storage CSI | Sakura CSI driver |

Rule: **persistent databases must never rely on `local-path` in production.** The CSI driver + cloud-controller-manager are installed during cluster bring-up (post-`terraform apply`, via Helm), not by Terraform, consistent with [ADR-014](../../adr/014-infrastructure-platform.md). CloudNativePG and Redis reference the cluster's CSI-backed `StorageClass` for their PVCs.

## Service Exposure Rules

Only the Gateway is publicly reachable. All other services are in-cluster only.

| Service | Exposure | Mechanism |
|---|---|---|
| Gateway | **Public** (WSS :443) | Traefik `Ingress` ← Hetzner LB (:80/:443) ← Cloudflare |
| Room Service | ClusterIP only | — |
| Game Server | ClusterIP only (headless for sticky routing) | — |
| PostgreSQL (CNPG) | ClusterIP only | read/write Services owned by CNPG |
| Redis | ClusterIP only | — |

Enforced jointly by K8s `Service` types, Traefik being the sole Ingress, and the `NetworkPolicy` set in `infra/k3s/network-policies.yaml` per [ADR-018 Security](../../adr/018-security.md). The Hetzner load balancer forwards **only** to the control-plane `:80/:443` (Traefik); 6443 stays private (agent join on the private network).

## Secret Management

**Now:** Kubernetes `Secret` per [ADR-018](../../adr/018-security.md). Secrets are created via `kubectl`/CI at deploy time; Terraform passes only non-secret config.

**Future migration path (documentation only):** adopt **External Secrets Operator** as the recommended bridge, syncing from a cloud-agnostic store:

- HashiCorp Vault, or
- 1Password Connect, or
- Cloud native secret managers (Hetzner has none; AWS Secrets Manager / Sakura equivalents).

Migration is non-disruptive: replace `Secret` manifests with `ExternalSecret` resources keeping the same `Secret` names and keys — workloads and Helm values reference unchanged names.

## Monitoring Roadmap

**Not deployed this phase.** The intended future stack (per [ADR-019 Observability](../../adr/019-observability.md) and the [Phase 5 spec](./phase-5-infra-as-code.md)):

| Component | Role |
|---|---|
| Prometheus | Scrapes `/metrics` on gateway/room-service/game-server via `ServiceMonitor` |
| Grafana | Dashboards (connections, tick duration, gRPC p50/p95/p99, entity count) |
| Loki + Promtail | Structured JSON log aggregation (`trace_id`, `request_id`) |
| Alertmanager | Alert routing (Slack/PagerDuty) |

These are platform components installed via the `infra/helm/monitoring` chart later; they are cloud-agnostic and consume the metrics/log schema already emitted by the services.

## Backup Strategy

**PostgreSQL (now, via CloudNativePG):** WAL archiving + base backups through Barman to an object store, enabling PITR. The destination is provider-specific object storage (Hetzner Storage Box / S3 / Sakura) configured per environment when `backups.enabled` is turned on.

**Kubernetes volumes (future, roadmap):** **Velero** for cluster resource + persistent-volume backup/restore, using the matching per-cloud Velero plugin (Hetzner / AWS / Sakura). This complements CNPG's database-level backups for full disaster recovery.

## K3s Upgrade Strategy (documentation only)

- **Workers (rolling):** one node at a time — `kubectl cordon` → `drain` (respecting PodDisruptionBudgets) → upgrade k3s agent → `uncordon`. Repeat across the pool. Data services (CNPG) maintain availability via replicas.
- **Control-plane:** single node in staging implies a brief maintenance window; in future HA mode, upgrade one server at a time after backing up state. Upgrade control-plane before workers (API version compatibility).
- **Maintenance windows:** schedule off-peak; announce ahead; verify `kubectl get nodes` post-upgrade.
- **Rollback:** keep the prior k3s package version available; re-run the installer pinned to the old version; CNPG/Redis retain their own replica-based rollback. Validate with the smoke test before closing the window.

## Environment Composition (staging)

`environments/staging/main.tf` composes only the contract + shared modules:

```hcl
resource "random_password" "k3s_token" {
  length  = 48
  special = false
}

module "cloud" {
  source        = "../../providers/hetzner"   # ← the line that selects the cloud
  cluster_name  = "spatial-staging"
  ssh_pub_key   = var.ssh_pub_key
  k3s_token     = random_password.k3s_token.result
  hcloud_token  = var.hcloud_token
  control_plane = { server_type = "cpx21", count = 1 }
  worker_pool = {
    server_type = "cpx31"
    count       = 2
    labels      = { workload = "game" }
    taints      = {}
  }
  allowed_ssh_cidrs = []
}

module "dns" {
  source               = "../../modules/dns"
  cloudflare_api_token = var.cloudflare_api_token
  zone_name            = var.dns_zone
  hostname             = "gateway.${var.dns_zone}"
  target               = module.cloud.load_balancer_endpoint
}
```

## Files Changed

### Create
- `infra/terraform/providers/shared/cloudinit/{versions,variables,main,outputs}.tf` — shared cloud-init rendering
- `infra/terraform/providers/hetzner/{versions,variables,main,outputs}.tf` — Hetzner impl of the contract
- `infra/terraform/modules/dns/{versions,variables,main,outputs}.tf` — Cloudflare (replaces Route53)
- `infra/helm/postgres/{Chart,values}.yaml` + `templates/{cluster,NOTES}.yaml` — CloudNativePG
- `infra/helm/redis/{Chart,values}.yaml` + `templates/NOTES.txt` — Bitnami

### Modify
- `infra/terraform/environments/staging/{versions,variables,main,backend}.tf` + `terraform.tfvars.example`
- `infra/cloud-init/k3s-server.yaml`, `k3s-agent.yaml` (pre-shared token + node label/taint args)
- `.github/workflows/terraform-plan.yml` (HCLOUD_TOKEN, CLOUDFLARE_API_TOKEN, TF_API_TOKEN)
- `Makefile` (terraform targets if needed)

### Delete (AWS-specific)
- `infra/terraform/providers/{aws.tf,versions.tf}`
- `infra/terraform/modules/{vpc,k3s,node_pool,rds,elasticache}/` (all)

### Documentation
- Amend [ADR-014](../../adr/014-infrastructure-platform.md): record Hetzner staging cloud + multi-cloud approach.
- Create [ADR-024 Multi-cloud Provider Abstraction](../../adr/024-multicloud-provider-abstraction.md): contract (incl. labels/taints), CloudNativePG-for-Postgres decision, node roles.
- Update [Phase 5 — Infra-as-Code](./phase-5-infra-as-code.md): Hetzner + Cloudflare + HCP backend + CNPG.

## Secrets & CI

| Secret | Used by | Source |
|---|---|---|
| `HCLOUD_TOKEN` | `providers/hetzner` | Hetzner Cloud API token |
| `CLOUDFLARE_API_TOKEN` | `modules/dns` | Cloudflare scoped token |
| `TF_API_TOKEN` | HCP Terraform backend | HCP Terraform user/team token |
| DB/Redis/JWT credentials | K8s Secrets (post-apply) / CNPG-generated | Not in Terraform ([ADR-018](../../adr/018-security.md)) |

Secrets are GitHub Actions secrets (CI) or local env; never committed.

## Validation & Testing

- `terraform fmt -check` and `terraform validate` per provider/shared module and per environment.
- `terraform plan` in CI (HCP Terraform workspace) on PRs touching `infra/terraform/`.
- `helm lint infra/helm/{postgres,redis}` after `helm dependency build`.
- Contract check: the input/output names + `worker_pool` shape are documented in ADR-024 so a future Sakura/AWS module is forced to match.
- Manual smoke (staging): `terraform apply` → install Hetzner CSI/CCM (real `hcloud` StorageClass) → `kubectl get nodes` (workers join, labeled `workload=game`) → `helm install` services + CNPG + redis → pods `Ready` → Gateway serves WSS through Hetzner LB + Cloudflare record.

## Rollout

Staging is greenfield — no data migration, no state import. Production (Sakura) is a future provider module under the same contract; if it later needs managed DBs, the Helm `managed` toggle + provider outputs provide the path. A dedicated production data-migration plan is out of scope here.

## Decisions Recap

| Decision | Choice | Rationale |
|---|---|---|
| PostgreSQL | **CloudNativePG** (operator) | K8s-native failover/PITR/rolling upgrades; cloud-agnostic |
| Redis | Bitnami | Mature, simple, portable |
| Worker scaling | Fixed-count pool, autoscaler-ready | Hetzner has no native ASG |
| Node scheduling | Contract `worker_pool.labels`/`.taints` | Workload isolation now, no contract change later |
| State backend | HCP Terraform (free) | Provider-independent |
| Structure | Provider modules + `shared/` + contract | One-line cloud swap; DRY cloud-init |
| DNS | Cloudflare | Survives cloud changes |
| Storage | Per-cloud CSI StorageClass; local-path dev-only | Portable charts; production-grade persistence |
| Service exposure | Gateway public (Traefik); all else ClusterIP | Least-privilege surface |
| K3s join | Pre-shared token | Fixes current placeholder; cloud-agnostic |
| Secrets | K8s Secret now; External Secrets future | Simple now, clear upgrade path |

## References

- [ADR-008 Deployment](../../adr/008-deployment.md) — K3s target, namespace, topology
- [ADR-014 Infrastructure Platform](../../adr/014-infrastructure-platform.md) — `infra/` layout, immutability, cloud agnosticism
- [ADR-017 Capacity Planning](../../adr/017-capacity-planning.md) — resource limits, ulimits
- [ADR-018 Security](../../adr/018-security.md) — NetworkPolicy isolation, Secret management
- [ADR-019 Observability](../../adr/019-observability.md) — metrics, logs, traces, dashboards
- [ADR-022 Session Management](../../adr/022-session-management.md) — Redis role
- [Phase 5 — Infra-as-Code](./phase-5-infra-as-code.md) — overall infra plan this builds on
- [CloudNativePG docs](https://cloudnative-pg.io/docs/) — operator + Cluster CR
