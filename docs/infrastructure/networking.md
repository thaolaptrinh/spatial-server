# Networking

> **Last Updated:** 2026-06-26

## Purpose

Document the network topology, segmentation, port allocation, firewall rules, and Kubernetes Network Policies for Spatial Server.

## Network Topology

Four isolated network segments (defined in ADR-012):

```
┌─────────────────────────────────────────────────────┐
│                  Public Network                       │
│  Client ↔ LB ↔ Gateway (WebSocket :443)             │
│  Operators ↔ Room Service (HTTP dashboard :8080)    │
└─────────────────────────────────────────────────────┘
                        │
┌─────────────────────────────────────────────────────┐
│                  Private Network                       │
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

## Segmentation Rules

| Network | Access | Direction | Purpose |
|---------|--------|-----------|---------|
| Public | External clients, operators | Inbound | WebSocket connections, management dashboard |
| Private | Internal services only | Bidirectional | Inter-service gRPC, never exposed externally |
| Database | Private-network services only | Inbound to DB ports | PostgreSQL and Redis access |
| Monitoring | All services push; operators query | Push from services, inbound from operators | Metrics, logs, dashboards |

## Port Allocation

| Service | Port | Protocol | Network | Description |
|---------|------|----------|---------|-------------|
| Gateway (client) | 443 | WSS | Public | Client WebSocket connections |
| Gateway (mgmt) | 8080 | HTTP | Public | Health check, metrics, pprof |
| Gateway (gRPC) | 9000 | gRPC | Private | Room Service and Game Server communication |
| Room Service (gRPC) | 9000 | gRPC | Private | Gateway and Game Server communication |
| Room Service (mgmt) | 8080 | HTTP | Public | Operator dashboard |
| Game Server (gRPC) | 9001 | gRPC | Private | Gateway, Room Service, and P2P Game Server communication |
| PostgreSQL | 5432 | TCP | Database | Primary database |
| Redis | 6379 | TCP | Database | Cache and pub/sub |
| Prometheus | 9090 | HTTP | Monitoring | Metrics collection |
| Grafana | 3000 | HTTP | Monitoring | Dashboards |
| Loki | 3100 | HTTP | Monitoring | Log aggregation |

## Firewall Rules (UFW)

Applied by cloud-init on each VM. See [cloud-init.md](cloud-init.md) for role-specific rules.

Default policy: deny all inbound, allow all outbound.

## Kubernetes Network Policies

Applied in the `spatial-server` namespace to enforce segmentation at the pod level:

```yaml
# Default deny all ingress
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
spec:
  podSelector: {}
  policyTypes:
  - Ingress

# Allow private network gRPC traffic
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-private-grpc
spec:
  podSelector:
    matchExpressions:
      - key: app
        operator: In
        values: [gateway, room-service, game-server]
  ingress:
  - from:
    - podSelector:
        matchExpressions:
          - key: app
            operator: In
            values: [gateway, room-service, game-server]
    ports:
    - protocol: TCP
      port: 9000
    - protocol: TCP
      port: 9001

# Allow database access from app pods
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-db-access
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/component: database
  ingress:
  - from:
    - podSelector:
        matchExpressions:
          - key: app
            operator: In
            values: [gateway, room-service, game-server]
    ports:
    - protocol: TCP
      port: 5432
    - protocol: TCP
      port: 6379
```

## CIDR Allocation (example)

| Environment | Public | Private | Database | Monitoring |
|-------------|--------|---------|----------|------------|
| Dev | `10.73.1.0/24` | `10.73.2.0/24` | `10.73.3.0/24` | `10.73.4.0/24` |
| Staging | `10.73.11.0/24` | `10.73.12.0/24` | `10.73.13.0/24` | `10.73.14.0/24` |
| Production | `10.73.21.0/24` | `10.73.22.0/24` | `10.73.23.0/24` | `10.73.24.0/24` |

## References

- ADR-012 — Networking
- ADR-014 — Infrastructure Platform
- [cloud-init.md](cloud-init.md)
- K3s NetworkPolicy docs
