# Terraform

> **Last Updated:** 2026-06-26

## Purpose

Provision all infrastructure (VMs, networking, DNS, volumes, load balancers) using Terraform as the single source of truth. Terraform never installs applications — that is the role of cloud-init and Helm.

## Providers

| Provider | Purpose | Source |
|----------|---------|--------|
| OpenStack / Proxmox | VM provisioning | `terraform-provider-openstack` / `bpg/proxmox` |
| Cloudflare (or equivalent DNS) | DNS records | `cloudflare/cloudflare` |

Providers are cloud-agnostic by design. The same Terraform structure works with any provider that offers VM, network, and DNS primitives.

## Modules

All modules live in `infra/terraform/modules/`:

| Module | Description |
|--------|-------------|
| `aegis-unit` | Standard VM unit: compute instance, floating IP (where applicable), security group, DNS record, cloud-init config rendering |
| `network` | VPC / private network segment, subnet, gateway |
| `dns` | DNS zone and A/AAAA records |

## Environments

| Environment | Directory | Description |
|-------------|-----------|-------------|
| dev | `infra/terraform/environments/dev/` | Single-VM dev environment (all services on one node) |
| staging | `infra/terraform/environments/staging/` | Multi-node staging cluster (smaller than prod) |
| production | `infra/terraform/environments/production/` | Full production cluster with HA and monitoring |

Each environment directory contains:

```
environments/<env>/
├── main.tf           # Root module calls
├── variables.tf      # Environment-specific variables
├── terraform.tfvars  # Variable values (secrets from secure store)
└── backend.tf        # State backend config (S3-compatible or local)
```

## Networking

Terraform provisions four network segments per environment:

| Network | CIDR (example) | Purpose |
|---------|----------------|---------|
| Public | `10.73.1.0/24` | Client-facing Gateway, operator management |
| Private | `10.73.2.0/24` | Inter-service gRPC |
| Database | `10.73.3.0/24` | PostgreSQL, Redis |
| Monitoring | `10.73.4.0/24` | Prometheus, Grafana, Loki |

See [networking.md](networking.md) for full topology.

## VM Provisioning

Each VM receives:
- CPU and RAM as per [k3s.md](k3s.md) Node Requirements
- A cloud-init configuration rendered from `infra/cloud-init/` templates
- Security group rules allowing only required ports per network segment
- A DNS A record in the internal zone

## State Management

Terraform state is stored in an S3-compatible backend (MinIO or cloud provider object storage) with DynamoDB-equivalent locking. State is never committed to the repository.

## References

- ADR-014 — Infrastructure Platform
- [k3s.md](k3s.md)
- [cloud-init.md](cloud-init.md)
- [networking.md](networking.md)
- [secrets.md](secrets.md)
