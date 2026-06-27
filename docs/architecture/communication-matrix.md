# Communication Matrix

> **Last Updated:** 2026-06-26

## Purpose

Define every communication path in the Spatial Server system — transport, protocol, direction, frequency, and latency requirements. This is the authoritative reference for who talks to whom, how, and under what constraints.

## Communication Matrix

| Source | Target | Transport | Protocol | Direction | Frequency | P99 Latency | Purpose |
|--------|--------|-----------|----------|-----------|-----------|-------------|---------|
| Client | Gateway | WSS (TLS 1.3) | Binary protobuf (length-prefixed) | Bidirectional | 20 Hz (game tick) | <100ms e2e | Realtime game state: position updates, entity spawn/despawn, actions |
| Business Backend | Room Service | gRPC (HTTP/2) | Protobuf (spatialserver.v1) | Unidirectional (BB → RS) | On-demand | <500ms | Runtime lifecycle: CreateRuntime, DestroyRuntime, GetRuntimeInfo |
| Gateway | Room Service | gRPC (HTTP/2) | Protobuf (spatialserver.v1) | Unidirectional (GW → RS) | Per-connection + cached | <10ms | Zone lookup: LookupZone, ReportMetrics |
| Gateway | Game Server | gRPC (HTTP/2) | Protobuf (spatialserver.v1) | Bidirectional | Per-client session | <10ms | Client packet forwarding: WebSocket ↔ gRPC bridge |
| Game Server | Room Service | gRPC (HTTP/2) | Protobuf (spatialserver.v1) | Unidirectional (GS → RS) | 5s (heartbeat) | <5ms | Control plane: Register, Heartbeat, PrepareShutdown, TransferZone, PrepareTransfer |
| Game Server | Game Server | gRPC (HTTP/2) | Protobuf (spatialserver.v1) | Bidirectional (P2P) | Event-driven | <5ms | Data plane: SendEntityUpdate, MigrateEntity, ZoneStateSync, QueryEntities |
| Game Server | PostgreSQL | TCP :5432 | pgx (PostgreSQL wire) | Bidirectional | 5s (persistence) | <2ms | Zone state persistence, crash recovery reads |
| Room Service | PostgreSQL | TCP :5432 | pgx (PostgreSQL wire) | Bidirectional | On change | <2ms | Zone ownership CRUD, Game Server registry |
| Gateway | Redis | TCP :6379 | go-redis (RESP) | Bidirectional | Per-connection | <1ms | Session cache, rate-limit counters |
| Room Service | Redis | TCP :6379 | go-redis (RESP) | Bidirectional | On change | <1ms | Metadata cache, pub/sub events |
| Game Server | Redis | TCP :6379 | go-redis (RESP) | Bidirectional | Event-driven | <1ms | Non-realtime pub/sub only |
| All Services | Prometheus | HTTP :9090 | Prometheus text format | Unidirectional (push) | 15s scrape | N/A | Metrics collection |
| All Services | Loki | HTTP :3100 | Protobuf (via Promtail) | Unidirectional (push) | Continuous | N/A | Structured log collection |

## Protocol Summary by Plane

### Data Plane (Latency-Critical)

| Path | Criticality | Loss Tolerance | Notes |
|------|-------------|----------------|-------|
| Client ↔ Gateway (WebSocket) | Highest | None | Real-time game experience |
| Gateway ↔ Game Server (gRPC) | Highest | None | Client data forwarding |
| Game Server ↔ Game Server (gRPC P2P) | High | Latest-wins | Entity sync between zones |

### Control Plane (Operational)

| Path | Criticality | Loss Tolerance | Notes |
|------|-------------|----------------|-------|
| Gateway ↔ Room Service | Medium | Cache covers outages | Zone lookups cached 5s TTL |
| Room Service ↔ Game Server | Medium | Heartbeat timeout handles | Registration, heartbeat, transfer (RPCs called by GS on RS) |
| Business Backend ↔ Room Service | Low | Retry on failure | Runtime create/destroy |

### Infrastructure Plane

| Path | Criticality | Loss Tolerance | Notes |
|------|-------------|----------------|-------|
| Service → Prometheus | Low | Metrics gap tolerated | Scrape failures self-correct |
| Service → Loki | Low | Log loss acceptable | Non-critical operational data |
| Service → PostgreSQL | High | Degraded mode available | In-memory fallback + write queue |

## Network Segmentation

```
Public Network:     Client ↔ Gateway (WSS :443)
Private Network:    All inter-service gRPC (:9000)
Database Network:   PostgreSQL (:5432), Redis (:6379)
Monitoring Network: Prometheus (:9090), Loki (:3100)
```

> **Control-plane RPC direction:** `Register`, `Heartbeat`, `PrepareShutdown`, `TransferZone`, and `PrepareTransfer` are defined on `RoomService` and called **by Game Server on Room Service** (GS → RS). Data-plane zone state flows via `GameServer.ZoneStateSync` (P2P between Game Servers, per ADR-002).

> **Gateway has no gRPC service.** The Gateway is a plain HTTP/WebSocket server. Client packet relay uses the `GameServer.Relay` bidi stream. The empty `Gateway` proto service is unused.

> **Load reporting:** `Heartbeat` carries no load parameter. Load reporting will be via `ReportMetrics` RPC (planned).

## Key Design Rules

- No service communicates directly with the Business Backend during gameplay
- Gateway never connects to PostgreSQL (stateless by design)
- Redis is NOT in the data path for realtime state (AOI, entity positions)
- Game Server ↔ Game Server P2P never routes through Room Service
- All monitoring traffic is push-only — no external entity queries services

## References

- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract (protobuf definitions)
- [ADR-012](../adr/012-networking.md) — Network Segmentation
- [Communication Patterns](communication.md) — Detailed communication flows
- [System Context](system-context.md) — External actor interactions
- [Component Responsibilities](component-responsibilities.md)
