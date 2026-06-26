# Deployment Diagram

> **Last Updated:** 2026-06-26

## Production Topology (K3s)

```mermaid
graph TB
    subgraph "Internet"
        CLIENTS[WebSocket Clients]
    end

    subgraph "Load Balancer"
        LB[L4 TCP Load Balancer<br/>Port 443]
    end

    subgraph "K3s Cluster"
        subgraph "Gateway Tier"
            GW1[Gateway Pod 1]
            GW2[Gateway Pod 2]
            GWN[Gateway Pod N]
        end

        subgraph "Coordinator Tier"
            RS1[Room Service Pod 1<br/>Leader]
            RS2[Room Service Pod 2<br/>Follower]
        end

        subgraph "Game Tier"
            GS1[Game Server Pod 1<br/>Zones A-C]
            GS2[Game Server Pod 2<br/>Zones D-F]
            GS3[Game Server Pod 3<br/>Zones G-I]
        end

        subgraph "Data Tier"
            PG1[(PostgreSQL<br/>Primary)]
            PG2[(PostgreSQL<br/>Replica)]
            RDS1[(Redis<br/>Primary)]
            RDS2[(Redis<br/>Replica)]
        end

        subgraph "Monitoring Tier"
            PROM[Prometheus]
            GRAF[Grafana]
            LOKI[Loki]
            PROMT[Promtail<br/>DaemonSet]
        end
    end

    CLIENTS -->|WSS :443| LB
    LB --> GW1
    LB --> GW2
    LB --> GWN

    GW1 -->|gRPC :9000| RS1
    GW1 -->|gRPC :9000| RS2
    GW2 -->|gRPC :9000| RS1
    GW2 -->|gRPC :9000| RS2

    GW1 -->|gRPC :9001| GS1
    GW1 -->|gRPC :9001| GS2
    GW1 -->|gRPC :9001| GS3
    GW2 -->|gRPC :9001| GS1
    GW2 -->|gRPC :9001| GS2
    GW2 -->|gRPC :9001| GS3

    RS1 -->|gRPC :9000| GS1
    RS1 -->|gRPC :9000| GS2
    RS1 -->|gRPC :9000| GS3
    RS2 -->|gRPC :9000| GS1
    RS2 -->|gRPC :9000| GS2
    RS2 -->|gRPC :9000| GS3

    GS1 <-->|gRPC P2P :9001| GS2
    GS1 <-->|gRPC P2P :9001| GS3
    GS2 <-->|gRPC P2P :9001| GS3

    GS1 -->|:5432| PG1
    GS2 -->|:5432| PG1
    GS3 -->|:5432| PG1
    RS1 -->|:5432| PG1
    RS2 -->|:5432| PG1

    GS1 -->|:6379| RDS1
    RS1 -->|:6379| RDS1

    GW1 --> PROM
    GW2 --> PROM
    GS1 --> PROM
    GS2 --> PROM
    GS3 --> PROM
    RS1 --> PROM
    RS2 --> PROM

    GW1 --> PROMT
    GW2 --> PROMT
    GS1 --> PROMT
    GS2 --> PROMT
    GS3 --> PROMT
    RS1 --> PROMT

    PROM --> GRAF
    PROMT --> LOKI
```

## Kubernetes Resource Mapping

| Resource | Purpose |
|----------|---------|
| **Namespace** | `spatial-server` |
| **Deployment** | Gateway (stateless), Room Service (2 replicas), Game Server (N replicas) |
| **StatefulSet** | PostgreSQL, Redis |
| **Service** | ClusterIP for internal gRPC, LoadBalancer/NodePort for WebSocket |
| **ConfigMap** | Application configs |
| **Secret** | JWT public key, DB credentials, TLS certs |
| **Ingress** | WebSocket path routing to Gateway (with TLS termination) |
| **HPA** | Auto-scale Game Server (CPU > 70%, memory > 80%) |
| **PDB** | PodDisruptionBudget for Game Servers (min available: 1) |

## Capacity Planning

| Environment | Spec | Services per Node |
|-------------|------|--------------------|
| Stage 1 (Single VM) | 2 vCPU, 4 GB RAM | All services on one node |
| Stage 2 (Separate GS) | 4 vCPU, 8 GB RAM per GS node | Game Server only |
| Stage 3 (Separate DB) | 2 vCPU, 8 GB RAM per DB node | PostgreSQL + Redis |
| Stage 4 (K3s Cluster) | 4 vCPU, 8 GB RAM per worker | Per K3s worker node |

## Per-Service Capacity Limits

| Service | Limit | Constraint |
|---------|-------|-----------|
| Gateway | 10,000 concurrent connections | File descriptors + goroutines |
| Game Server | 5,000 entities (100/zone × 50 zones) | AOI query complexity O(n log n) |
| Room Service | 100 Game Servers registered | In-memory map + heartbeat processing |
| PostgreSQL | 50 concurrent connections | Pooled via pgx (max 20 per service) |

## References

- [Infrastructure Overview](../infrastructure/overview.md)
- [Deployment Guide](../operations/deployment.md)
- [ADR-008](../adr/008-deployment.md) — Deployment Strategy
- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning
