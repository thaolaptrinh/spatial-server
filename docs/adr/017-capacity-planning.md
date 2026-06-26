# ADR 017: Capacity Planning

## Status

Accepted

## Context

Spatial Server must define capacity targets per service to guide infrastructure sizing, autoscaling thresholds, and hardware provisioning. Without defined limits, it is impossible to distinguish normal operation from capacity incidents.

## Decision

### Per-Node Baseline

Initial production hardware baseline:
- 2 vCPU
- 4 GB RAM
- 100 Mbps network

This baseline is for a single K3s node hosting multiple services. It is the minimum viable production target, not the recommended long-term configuration.

### Gateway Capacity

| Metric | Target | Constraint |
|--------|--------|------------|
| Concurrent WebSocket connections | 10,000 per instance | File descriptors + goroutine overhead |
| Message throughput | 1,000 msg/s per connection | Rate limited (100 msg/s per connection) |
| Bandwidth per player | 50-200 Kbps | Depends on update frequency and entity density |
| Memory per connection | ~50 KB | Session state + read/write buffers |

### Game Server Capacity

| Metric | Target | Constraint |
|--------|--------|------------|
| Entities per zone | 100 | AOI query complexity ~ O(n log n) in-zone |
| Zones per Game Server | 50 | Memory for in-memory AOI index + entity state |
| Total entities per Game Server | 5,000 | 100 entities/zone × 50 zones |
| Memory per entity | ~5 KB | Position, attributes, AOI subscriptions |
| Tick rate | 20Hz (50ms window per tick) | Game loop must complete within tick budget |

### Room Service Capacity

| Metric | Target | Constraint |
|--------|--------|------------|
| Registrations | 100 Game Servers | In-memory map + heartbeat processing |
| Zone lookups | 10,000 req/s | PostgreSQL query + cache |
| Runtimes managed | 1,000 | Lightweight — metadata only |

### Redis Capacity

| Metric | Target | Constraint |
|--------|--------|------------|
| Session cache | 100,000 entries | Key-value, ~100 bytes per entry |
| Metadata cache | 10,000 keys | Runtime + zone + Game Server metadata |

### PostgreSQL Capacity

| Metric | Target | Constraint |
|--------|--------|------------|
| Concurrent connections | 50 | Pooled via pgx (max 20 per service) |
| Zone ownership writes | 1,000/s | Indexed by zone_id, lightweight |
| Runtime operations | 100/s | Create/destroy runtime |

### Scaling Thresholds

| Metric | Threshold | Action |
|--------|-----------|--------|
| Gateway CPU | >70% for 30s | Add Gateway replica |
| Gateway connections | >8,000 (80% of 10K) | Add Gateway replica |
| Game Server CPU | >70% for 30s | Add Game Server replica |
| Game Server memory | >80% for 30s | Add Game Server replica |
| Zone imbalance | stddev > 30% | Rebalance (no new server) |
| PostgreSQL connections | >40 (80% of pool) | Add PgBouncer or increase pool |
| Redis memory | >80% of maxmemory | Scale up or enable eviction |

## Consequences

- Hardware baseline of 2 vCPU / 4 GB RAM constrains initial deployment.
- Gateway at 10K connections is the first bottleneck to hit (file descriptors, goroutines).
- Game Server at 5,000 entities across 50 zones before needing another replica.
- Scaling thresholds are deliberately conservative (70% CPU triggers action, not 90%).
- These are estimates — benchmarks in Phase 2/3 will validate or revise them.

## Replaces

- None. This is the first formal capacity plan.
