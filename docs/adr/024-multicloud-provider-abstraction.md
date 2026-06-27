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
