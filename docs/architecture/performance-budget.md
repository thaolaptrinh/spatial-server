# Performance Budget

> **Last Updated:** 2026-06-26

## Purpose

Defines quantitative performance targets for each service. These budgets serve as acceptance criteria — no phase is complete until targets are measured and met.

## Global Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| End-to-end latency | <100ms p95 | Client → Gateway → Game Server → Gateway → Client |
| Internal RPC latency | <5ms p99 | Direct Game Server ↔ Game Server within same DC |
| Tick rate | 20Hz (50ms window) | Game loop must complete within tick budget |
| Packet loss | <0.1% | Client to Gateway, measured at LB level |
| Connection success rate | >99.9% | WebSocket handshake completion rate |

## Per-Service Budgets

### Gateway

| Metric | Target | Degradation Threshold | Constraint |
|--------|--------|----------------------|------------|
| Concurrent connections | 10,000 per instance | >8,000 triggers scale-out | File descriptors + goroutine overhead |
| Message throughput | 1,000 msg/s per connection | Rate-limited at 100 msg/s per connection | Bandwidth 50-200 Kbps per player |
| Connection rate | 500 conn/s per instance | Burst up to 1,000 conn/s | Handshake + JWT validation |
| Memory per connection | ~50 KB | Accept up to 80% of instance RAM | Session state + read/write buffers |
| P99 Auth latency | <50ms | JWT validation + rate limit check | Public key crypto (ECDSA) |
| P99 Lookup latency | <10ms | Zone lookup in cached routing table | Cache hit (TTL 5s) |
| Uptime | 99.9% | Stateless, behind LB | Roll restart safe |

### Game Server

| Metric | Target | Degradation Threshold | Constraint |
|--------|--------|----------------------|------------|
| Entities per zone | 100 | >100 degrades AOI query performance | AOI complexity O(n log n) in-zone |
| Zones per server | 50 | >50 exceeds memory budget | In-memory AOI index + entity state |
| Total entities per server | 5,000 | >5,000 triggers scale-out | 100/zone × 50 zones |
| Memory per entity | ~5 KB | Monitor for leaks via pprof | Position, attributes, AOI subscriptions |
| Tick budget | 50ms (20Hz) | >50ms × 10 consecutive = warning alert | Game loop must complete all phases |
| P99 AOI query | <5ms | >10ms triggers investigation | Grid-based spatial index |
| Zone transfer | <2s per zone | <5s for largest zone states | Serialized snapshot + gRPC stream |
| P99 ZoneStateSync | <30s | For full server state transfer | Stream of multiple zones |
| Uptime | 99.95% | Per-instance; zone transfer mitigates crashes | PodDisruptionBudget: min 1 available |

### Room Service

| Metric | Target | Degradation Threshold | Constraint |
|--------|--------|----------------------|------------|
| Game Server registrations | 100 | >100 investigate before scaling | In-memory map + heartbeat processing |
| Zone lookups | 10,000 req/s | Cache-miss rate under 1% | PostgreSQL query |
| Runtimes managed | 1,000 | Metadata only — lightweight | PostgreSQL table size |
| P99 LookupZone | <5ms | >20ms triggers investigation | Cache hit expected on hot path |
| P99 CreateRuntime | <500ms | >2s triggers investigation | Zone allocation + DB writes |
| P99 Heartbeat processing | <1ms | Batch per tick | 100 servers × 1 heartbeat/5s = 20/s |
| Failover | <5s | Leader election via K3s Lease API | Follower is warm standby |
| Uptime | 99.95% | HA pair | Gateway cache covers short outages |

### PostgreSQL

| Metric | Target | Constraint |
|--------|--------|------------|
| Concurrent connections | 50 | Pooled via pgx (max 20 per service) |
| Zone ownership writes | 1,000/s | Indexed by zone_id, lightweight |
| Runtime operations | 100/s | Create/destroy runtime |
| Query latency (hot path) | <2ms p99 | Primary: simple PK lookups |
| Replication lag | <100ms | Streaming replication to read replicas |

### Redis

| Metric | Target | Constraint |
|--------|--------|------------|
| Session cache entries | 100,000 | Key-value, ~100 bytes per entry |
| Metadata cache keys | 10,000 | Runtime + zone + Game Server metadata |
| Cache hit rate | >95% | Validated via `info stats` |
| Pub/sub throughput | 1,000 msg/s | Non-realtime events only |

## Throughput Model

| Scale Level | Gateways | Game Servers | Concurrent Players | Connections per Gateway | Entities per Server |
|-------------|----------|--------------|--------------------|------------------------|---------------------|
| Small (dev) | 1 | 1 | 100 | 100 | 500 |
| Medium (staging) | 2 | 3 | 5,000 | 2,500 | 1,667 |
| Large (Phase 1 prod) | 5 | 10 | 50,000 | 10,000 | 5,000 |
| Extra Large (Phase 3) | 20+ | 40+ | 200,000+ | 10,000 | 5,000 |

## Degradation Strategy

When any service breaches its performance budget, the following actions are taken in order:

1. **Drop non-critical updates** — position interpolation, cosmetic animations
2. **Reduce broadcast frequency** — AOI updates skip every Nth tick (graceful degradation)
3. **Disconnect abusive clients** — rate limit violators disconnected
4. **Never block the game loop** — all backpressure mechanisms must be non-blocking

## Measurement

- All services expose Prometheus histograms for latency metrics
- Tick duration is the primary Game Server health indicator
- Benchmark scenarios run before every production deployment (see ADR-020)
- Per-RPC latency tracked: p50, p95, p99, p99.9
- Memory profiling via `pprof` during load tests

## References

- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning (per-service limits)
- [ADR-020](../adr/020-benchmark-strategy.md) — Benchmark Strategy
- [ADR-019](../adr/019-observability.md) — Observability (metrics & alerting)
- [Overview](overview.md) — Technology stack
