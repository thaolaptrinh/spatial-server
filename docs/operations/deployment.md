# Deployment Guide

> **Last Updated:** 2026-06-26

## Purpose

Describe the deployment procedures for Spatial Server across environments.

## Environments

### Local Development

```
Platform: Docker Compose (single host, no K3s)
Services: gateway, room-service, game-server (1 replica each)
DB:       PostgreSQL + Redis (Docker containers)
Run:      docker compose -f deploy/docker-compose/docker-compose.yml up
Config:   configs/*.yml + environment variables
```

**Note:** Docker Compose exists ONLY for local development. Production architecture is always K3s.

### Staging

```
Same as Production topology (K3s), smaller resource allocation.
- Fewer Game Server replicas
- Single PostgreSQL replica
- Prometheus + Grafana + Loki for observability
```

### Production (K3s)

```
Infrastructure Flow:
Terraform → cloud-init → Docker → K3s → Helm → Spatial Server Services
```

## Deployment Principles

- Infrastructure is immutable. Servers are never configured manually.
- Everything reproducible from source code (Terraform + cloud-init + Helm).
- SSH is for debugging only, never for deployment.
- Terraform provisions VMs, networking, DNS, volumes, LBs — never installs apps.
- cloud-init bootstraps new servers: Docker, K3s, cluster join.
- Helm charts exist for every deployable component.
- Secrets never in source code. Kubernetes Secret + env vars.
- Logs are structured JSON with trace_id, request_id, correlation_id.

## CI/CD Pipeline

```
┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌─────────┐
│   Lint   │ → │  Unit    │ → │  Build   │ → │  Docker  │ → │ Release │
│ (golang- │   │  Test    │   │ (go build)│   │  Build   │   │ (tag)   │
│  ci-lint)│   │          │   │          │   │          │   │         │
└──────────┘   └──────────┘   └──────────┘   └──────────┘   └─────────┘
```

### CI Config (GitHub Actions)

```yaml
on: [push, pull_request]
jobs:
  lint:
    run: golangci-lint run ./...
  unit-test:
    run: go test ./pkg/... -race -coverprofile=coverage.out
  build:
    run: go build ./apps/...
  docker:
    run: docker build -f deploy/docker/gateway.Dockerfile -t gateway:$TAG .
  integration-test:
    run: docker compose -f deploy/docker-compose/docker-compose.yml up -d && go test ./test/integration/...
```

## Tag Strategy

| Tag | Purpose |
|-----|---------|
| `dev` | Latest on main branch |
| `staging` | Git tag with `-staging` suffix |
| `production` | Semantic version (`v1.0.0`, `v1.1.0`) |

## References

- [Infrastructure Guide](../infrastructure/overview.md)
- [ADR-008](../adr/008-deployment.md) — Deployment Strategy
- [ADR-014](../adr/014-infrastructure-platform.md) — Infrastructure Platform
