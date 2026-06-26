# ADR 014: Infrastructure Platform

## Status

Accepted

## Context

Spatial Server is designed as a production-ready distributed realtime platform. Infrastructure must be: production ready, Infrastructure as Code, cloud agnostic, horizontally scalable, reproducible, low complexity, and easy for local development. Infrastructure decisions must be finalized before implementation begins.

## Decision

### Stack

| Layer | Technology |
|---|---|
| Infrastructure as Code | Terraform |
| VM Bootstrap | cloud-init |
| Configuration Management | cloud-init (default), Ansible (optional) |
| Container Runtime | Docker |
| Container Orchestrator | K3s |
| Package Manager | Helm |
| CI/CD | GitHub Actions |
| Monitoring | Prometheus |
| Dashboard | Grafana |
| Logging | Loki + Promtail |
| Tracing | OpenTelemetry |
| Secret Management | Kubernetes Secret |
| Configuration | ConfigMap |
| Database | PostgreSQL |
| Cache | Redis |

### Infrastructure Repository

```
infra/
├── terraform/
│   ├── providers/
│   ├── modules/
│   └── environments/
├── helm/
│   ├── gateway/
│   ├── room-service/
│   ├── game-server/
│   ├── redis/
│   ├── postgres/
│   └── monitoring/
├── cloud-init/
│   ├── gateway/
│   ├── room-service/
│   └── game-server/
├── docker/
│   └── compose/
├── scripts/
└── docs/
```

### Principles

- Infrastructure is immutable. Servers are never configured manually.
- Everything must be reproducible from source code.
- SSH is for debugging only, never for deployment.
- Terraform provisions VMs, networking, DNS, volumes, load balancers — but never installs applications.
- cloud-init bootstraps new servers: Docker, K3s, SSH config, cluster join, hostname.
- Production uses K3s. Docker Compose exists only for local development.
- Every deployable component must have a Helm Chart.
- Secrets never in source code. Use Kubernetes Secret + env vars.
- Logs are structured JSON with trace_id, request_id, correlation_id.
- OpenTelemetry is the standard tracing solution.
- Cloud agnostic — no business logic depends on a specific cloud provider.

## Consequences

- Phase 1 must include Helm charts alongside Docker Compose.
- Terraform configs are created in Phase 4 (production hardening) but designed from the start.
- No etcd, Consul, or external service discovery — K3s built-in Services suffice.
- Cluster Autoscaler is future consideration, not Phase 1.
- infra/ directory lives in the same repository (monorepo) for Phase 1-3.

## Replaces

- Previous design had K3s as "future reference". Now it is the production baseline from day one.
- Previous design did not define Terraform, Helm, cloud-init, or Loki.
