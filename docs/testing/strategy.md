# Testing Strategy

> **Last Updated:** 2026-06-26

## Purpose

Define the testing approach for Spatial Server at all levels.

## Test Levels

| Type | Scope | Tool | Frequency |
|------|-------|------|-----------|
| **Unit** | Individual packages | `go test`, table-driven | Every commit |
| **Integration** | Service + DB (PostgreSQL, Redis) | `tests/integration/` with Testcontainers | Every PR |
| **Load** | Gateway + Game Server under load | k6 + custom WebSocket client | Per milestone |
| **Chaos** | Network partitions, crash recovery | `tests/chaos/` (scripted) | Pre-release |
| **Benchmark** | AOI queries, RPC serialization | `go test -bench=.` | Per commit |

## Test Targets by Service

### Gateway
- Connection handling
- Rate limiting
- JWT authentication
- Packet validation (encoding/decoding)
- WebSocket upgrade

### Room Service
- Zone ownership (claim, transfer, release)
- Load balancing (server selection)
- Heartbeat timeout
- Leader election

### Game Server
- Entity simulation
- AOI queries (grid-based)
- State persistence
- Zone transfer (serialize, load)
- Position update broadcast

### RPC
- Timeout (per-RPC configurable)
- Retry (exponential backoff)
- Idempotency
- Streaming (zone state sync)

### Protocol
- Packet encoding/decoding
- Compression
- Sequence number validation (replay protection)
- Version negotiation

## Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Concurrent connections per Gateway | 10,000 | Limited by file descriptors + goroutines |
| Concurrent connections total | 50,000 (Phase 1), 200,000+ (Phase 3) | Horizontally scalable |
| Players per zone | 100 | AOI query cost grows with player count |
| End-to-end latency | <100ms p95 | Client → Gateway → Game Server → Gateway → Client |
| Internal RPC latency | <5ms p99 | Direct Game Server ↔ Game Server |
| Tick rate | 20Hz (configurable) | 50ms per tick |
| CPU per Game Server | 1 core per 500 entities | Estimate; benchmark to validate |
| Memory per entity | ~5 KB | Position, attributes, AOI subscriptions |
| Bandwidth per player | 50-200 Kbps | Depends on update frequency and entity density |

## CI Gate

- Light load scenario runs on every PR
- Any latency regression >10% blocks merge
- Full benchmark suite per milestone before production deployment

## References

- [ADR-020](../adr/020-benchmark-strategy.md) — Benchmark Strategy
- [Benchmark Scenarios](benchmark-scenarios.md)
- [Unit Testing](unit-testing.md)
- [Integration Testing](integration-testing.md)
- [Load Testing](load-testing.md)
- [Simulation Testing](simulation-testing.md)
- [Chaos Testing](chaos-testing.md)
