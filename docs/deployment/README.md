# Deployment

> **Last Updated:** 2026-06-26

## Purpose

Deployment configuration and environment documentation for Spatial Server.

## Contents

| Document | Description |
|----------|-------------|
| [Deployment Guide](../operations/deployment.md) | Full deployment procedures for all environments |
| [Docker Compose](../infrastructure/docker-compose.md) | Local development with Docker Compose |
| [K3s](../infrastructure/k3s.md) | Production K3s cluster setup |
| [Helm](../infrastructure/helm.md) | Helm chart structure and conventions |
| [Terraform](../infrastructure/terraform.md) | Infrastructure as Code with Terraform |
| [cloud-init](../infrastructure/cloud-init.md) | VM bootstrap configuration |
| [Secrets](../infrastructure/secrets.md) | Secrets management strategy |
| [Networking](../infrastructure/networking.md) | Network topology and firewall rules |
| [Deployment Diagram](../diagrams/deployment.md) | Production deployment topology diagram |

## Environment Summary

| Environment | Orchestrator | Nodes | TLS | Monitoring | Purpose |
|-------------|-------------|-------|-----|------------|---------|
| Local Dev | Docker Compose | 1 | No | No | Fast iteration, hot reload |
| Staging | K3s | 1-3 | Optional | Full stack | Pre-production validation, load testing |
| Production | K3s | 3+ | Required | Full stack | Live user traffic |

## Reading Order

1. Start with the [Deployment Guide](../operations/deployment.md) for the full deployment pipeline.
2. Review [Docker Compose](../infrastructure/docker-compose.md) for local development.
3. Study [K3s](../infrastructure/k3s.md) and [Helm](../infrastructure/helm.md) for production deployment.
4. Consult [Terraform](../infrastructure/terraform.md) and [cloud-init](../infrastructure/cloud-init.md) for infrastructure provisioning.
5. Review the [Deployment Diagram](../diagrams/deployment.md) for visual topology.

## Related ADRs

- [ADR-008: Deployment Strategy](../adr/008-deployment.md)
- [ADR-014: Infrastructure Platform](../adr/014-infrastructure-platform.md)
