# ADR 012: Networking

## Status

Approved

## Context

The platform has multiple network planes: public (clients), private (inter-service), database, and monitoring. Each has different security and performance requirements.

## Problem

Multiple services with different network requirements need clear network boundaries. Public-facing services require exposure while internal services must be isolated. Without a clear network topology, security vulnerabilities and configuration errors arise.

## Decision

### Network Topology

```
Public Network
  Client ↔ LB ↔ Gateway (WebSocket :443, WSS)
  Operators ↔ Room Service (HTTP management :8080)
        │
Private Network
  Gateway ↔ Room Service (gRPC :9000)
  Gateway ↔ Game Server (gRPC :9001)
  Room Service ↔ Game Server (gRPC :9000)
  Game Server ↔ Game Server (gRPC :9001, direct P2P)
        │
Database Network
  All services → PostgreSQL (:5432)
  All services → Redis (:6379)
        │
Monitoring Network
  All services → Prometheus (:9090)
  All services → Grafana (:3000)
  All services → Loki (:3100)
```

### Rules

- **Public Network**: Only Gateway and Room Service management port exposed.
- **Private Network**: All inter-service gRPC. No external access. mTLS optional (Phase 4).
- **Database Network**: PostgreSQL + Redis accessible only from private network services.
- **Monitoring Network**: Agent-sidecar push only — services push metrics, cannot be queried from outside.

### Port Allocation

| Service | Port | Protocol | Network |
|---------|------|----------|---------|
| Gateway (client) | 443 | WebSocket (WSS) | Public |
| Gateway (mgmt) | 8080 | HTTP | Public (operators) |
| Gateway (gRPC) | 9000 | gRPC | Private |
| Room Service (gRPC) | 9000 | gRPC | Private |
| Room Service (mgmt) | 8080 | HTTP | Public |
| Game Server (gRPC) | 9001 | gRPC | Private |
| PostgreSQL | 5432 | TCP | Database |
| Redis | 6379 | TCP | Database |

## Alternatives

1. **Flat network (all services accessible)**: Simplest to configure but highest security risk — any compromised service exposes all others.
2. **Service mesh (Istio/Linkerd)**: Fine-grained network control with automatic mTLS and observability. High operational complexity for an MVP.
3. **Network policies only (K3s NetworkPolicy)**: Software-defined network isolation at the pod level. Effective but K3s-only, not portable to Docker Compose.

## Tradeoffs

- Four network planes provide strong isolation but add configuration overhead across deployment targets.
- Push-based monitoring avoids open firewall ports but requires agent sidecars on each service.
- Consistent port assignments simplify debugging and documentation but reduce flexibility for port conflicts.

## Consequences

- Clear network boundaries reduce attack surface.
- gRPC traffic never traverses public network.
- Monitoring is push-only (no open ports on services for Prometheus scraping).
- Port assignments are consistent across all deployment targets.

## Future Considerations

- mTLS for all inter-service gRPC communication.
- eBPF-based network observability for performance monitoring.
- Network policy-as-code for audit and compliance.

## Replaces

None.
