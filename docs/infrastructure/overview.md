# Infrastructure Overview

> **Last Updated:** 2026-06-26

## Purpose

Document the infrastructure stack and configuration for Spatial Server deployments.

## Stack

| Layer | Technology |
|-------|-----------|
| Infrastructure as Code | Terraform |
| VM Bootstrap | cloud-init |
| Configuration Management | cloud-init (default), Ansible (optional) |
| Container Runtime | Docker |
| Container Orchestrator | K3s (production), Docker Compose (local dev) |
| Package Manager | Helm |
| CI/CD | GitHub Actions |
| Monitoring | Prometheus + Grafana + Loki + Promtail |
| Tracing | OpenTelemetry |
| Secret Management | Kubernetes Secret |
| Configuration | ConfigMap |

## K3s (Kubernetes) Architecture (Production Baseline)

| Resource | Purpose |
|----------|---------|
| **Namespace** | `spatial-server` |
| **Deployment** | Gateway (stateless), Room Service (2 replicas, HA), Game Server (N replicas) |
| **StatefulSet** | PostgreSQL, Redis |
| **Service** | ClusterIP for internal gRPC, LoadBalancer/NodePort for WebSocket |
| **ConfigMap** | Application configs (gateway.yml, room-service.yml, game-server.yml) |
| **Secret** | JWT public key, DB credentials, TLS certs |
| **Ingress** | WebSocket path routing to Gateway (with TLS termination) |
| **HPA** | Auto-scale Game Server based on CPU, memory, and custom Prometheus metrics |
| **PDB** | PodDisruptionBudget for Game Servers (min available: 1) |
| **Node Affinity** | Game Servers prefer different nodes for fault isolation |
| **Anti Affinity** | Game Servers should NOT co-locate on same node |

## Networking

```
┌─────────────────────────────────────────────────────┐
│                  Public Network                       │
│  Client ↔ LB ↔ Gateway (WebSocket :443)             │
│  Operators ↔ Gateway (HTTP mgmt :8080)              │
│  Operators ↔ Room Service (HTTP dashboard :8080)    │
└─────────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────────┐
│                  Private Network                      │
│  Gateway ↔ Room Service (gRPC :9000)                │
│  Gateway ↔ Game Server (gRPC :9001)                 │
│  Room Service ↔ Game Server (gRPC :9001)            │
│  Game Server ↔ Game Server (gRPC :9001)             │
└─────────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────────┐
│                  Database Network                     │
│  All services ↔ PostgreSQL (:5432)                  │
│  All services ↔ Redis (:6379)                       │
└─────────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────────┐
│                 Monitoring Network                    │
│  All services → Prometheus (:9090)                   │
│  All services → Grafana (:3000)                      │
│  All services → Loki (:3100)                         │
└─────────────────────────────────────────────────────┘
```

### Segmentation Rules

- Public Network: only Gateway and Room Service management port
- Private Network: all inter-service gRPC, no external access
- Database Network: PostgreSQL + Redis, only accessed by private network services
- Monitoring Network: agent-sidecar push only (services push, cannot be queried from outside)

## Configuration Strategy

| Config Type | Mechanism | Example |
|-------------|-----------|---------|
| Environment Variables | `os.Getenv()` | `DB_URL`, `REDIS_ADDR`, `JWT_SECRET` |
| YAML Config Files | koanf | `configs/game-server.yml` |
| Secrets | Environment variables + volume mounts | TLS certs, DB passwords |
| Dynamic Config | PostgreSQL watched keys | Zone size, tick rate, AOI radius |

**Precedence (high → low):**
1. CLI flags
2. Environment variables
3. Dynamic config (DB/Redis)
4. Config files
5. Defaults

## References

- [Deployment Guide](../operations/deployment.md)
- [Docker Compose](docker-compose.md)
- [K3s](k3s.md)
- [Terraform](terraform.md)
- [Helm](helm.md)
- [cloud-init](cloud-init.md)
- [Secrets](secrets.md)
- [Networking](networking.md)
- [ADR-014](../adr/014-infrastructure-platform.md) — Infrastructure Platform
- [ADR-008](../adr/008-deployment.md) — Deployment Strategy
