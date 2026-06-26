# Infrastructure Documentation

> **Last Updated:** 2026-06-26

## Purpose

Define the deployment infrastructure for Spatial Server — Docker Compose for local development, K3s for production, and the infrastructure-as-code pipeline (Terraform, cloud-init, Helm).

## Contents

| File | Description |
|------|-------------|
| [overview.md](overview.md) | Infrastructure overview — environments, network segmentation, and services |
| [docker-compose.md](docker-compose.md) | Docker Compose configuration for local development |
| [k3s.md](k3s.md) | K3s cluster architecture — nodes, namespaces, and resource allocation |
| [terraform.md](terraform.md) | Terraform modules for cloud infrastructure provisioning |
| [helm.md](helm.md) | Helm chart structure and deployment configuration |
| [cloud-init.md](cloud-init.md) | cloud-init templates for node bootstrapping |
| [networking.md](networking.md) | Network topology, port allocation, firewall rules, and Kubernetes NetworkPolicies |
| [secrets.md](secrets.md) | Secret management strategy and vault configuration |

## Reading Order

1. Start with [overview.md](overview.md) for the infrastructure landscape.
2. Read [networking.md](networking.md) for network topology and port allocation.
3. Study [k3s.md](k3s.md) and [helm.md](helm.md) for production deployment structure.
4. Review [terraform.md](terraform.md) and [cloud-init.md](cloud-init.md) for infrastructure provisioning.
5. Consult [secrets.md](secrets.md) for security and secret management.

## Related Documents

- [Operations](../operations/README.md) — Deployment, runbook, and operational procedures
- [Architecture](../architecture/README.md) — System architecture
- [ADR-008](../adr/008-deployment.md) — Deployment ADR
- [ADR-012](../adr/012-networking.md) — Networking ADR
