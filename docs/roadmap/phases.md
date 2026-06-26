# Development Roadmap

> **Last Updated:** 2026-06-26

## Phase Overview

| Phase | Duration | Focus | Status |
|-------|----------|-------|--------|
| Phase 0 | 3–5 days | Architecture & specs (ADRs) | ✅ Complete |
| Phase 1 | 2 weeks | Core infrastructure | 🔧 In Progress |
| Phase 2 | 2 weeks | Realtime features | 📋 Planned |
| Phase 3 | 3 weeks | Distributed scaling | 📋 Planned |
| Phase 4 | 2 weeks | Production hardening | 📋 Planned |

## Phase 0: Architecture (Complete)

```
- [x] Reference research (GoWorld, Pitaya)
- [x] Architecture design document
- [x] Team review and approval
- [x] 20 ADRs created (001–020)
- [x] Implementation plan created
```

## Phase 1: Core Infrastructure (In Progress)

**Deliverables:**
- Project scaffold with Makefile, go.mod, directory structure
- PostgreSQL schema (runtimes, zones, zone_ownership, game_servers)
- Redis connection layer
- Configuration loading (koanf)
- Structured logging (slog)
- gRPC protobuf definitions for all services
- Gateway: WebSocket acceptor + JWT auth + rate limiting
- Room Service: zone ownership CRUD + heartbeat + health check
- Game Server: basic loop + entity model + position storage
- Docker Compose for local dev
- CI: lint + unit test + build
- golang-migrate for database migrations
- UUIDv7 entity ID generation

**Non-goals:**
- AOI
- Zone transfer
- Multiple Game Servers

## Phase 2: Realtime Features (Planned)

**Deliverables:**
- Grid-based AOI system with in-memory index
- Position sync: client → Gateway → Game Server → Gateway → interested clients
- Entity spawn/despawn in AOI range
- Zone boundary crossing with ghost entity timeout
- Direct Game Server ↔ Game Server RPCs
- Session management and reconnection
- Packet protocol: compression, sequence numbers, replay protection
- Simulation framework (v1 for basic load testing)

**Non-goals:**
- Multiple Game Servers
- Zone transfer
- Auto-scaling

## Phase 3: Distributed Scaling (Planned)

**Deliverables:**
- Multiple Game Server support
- Room Service zone ownership table + assignment
- Zone transfer between Game Servers
- Gateway routing by zone (lookup → cache → forward)
- Game Server heartbeat + crash recovery
- Room Service leader election (Kubernetes Lease API)
- Load-based zone rebalancing
- Prometheus metrics + Grafana dashboards
- Simulation framework (full capabilities)

**Non-goals:**
- Kubernetes
- Production hardening

## Phase 4: Production Hardening (Planned)

**Deliverables:**
- K8s manifests (reference, not full implementation)
- HPA configuration (custom metrics)
- Load testing + optimization (via simulation framework)
- Chaos testing (partition, crash, latency injection)
- LZ4/Snappy packet compression
- TLS for WebSocket
- mTLS for internal gRPC (optional)
- Production-ready K3s manifests
- Monitoring alerts (PagerDuty / Slack)
- Documentation for API consumers
- Benchmarking report

## References

- [Architecture Overview](../architecture/overview.md)
- [ADR Index](../adr/README.md)
