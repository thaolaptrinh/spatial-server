# Scaling Guide

> **Last Updated:** 2026-06-26

## Purpose

Document when and how to scale each service, pre-scaling for known events, capacity planning verification, and vertical vs. horizontal scaling decisions.

## Scaling Philosophy

Horizontal scaling first, vertical scaling only when horizontal is infeasible. See [ADR-015](../adr/015-architecture-principles.md) §2.

| Service | Strategy | Reason |
|---------|----------|--------|
| Gateway | Horizontal (stateless) | No session affinity. Add/remove behind LB. |
| Game Server | Horizontal (coordinator-managed) | Room Service manages zone redistribution. |
| Room Service | Vertical (leader election) | Active/passive pair. Single leader handles all coordination. |
| PostgreSQL | Vertical (primary) / Horizontal (read replicas) | Writes hit primary. Reads can scale with replicas. |
| Redis | Vertical (primary) / Horizontal (cluster mode) | Session cache scales vertically. Cluster for >100K keys. |

## Service Scaling

### Gateway — Horizontal

**When to scale:**
- Connections per instance > 8,000 (80% of 10K capacity).
- CPU > 70% for 30s.
- Memory > 80% for 30s.

**How to scale:**
- K8s: HPA with custom metric `websocket_connections` (Prometheus adapter).
- Docker Compose: `docker compose scale gateway=N`.
- Room Service discovers new Gateway via service registration.
- No session affinity needed — clients reconnect to any Gateway via session token.

**Scale-down:** Gradual. Drain connections (tell clients to reconnect to other Gateways), wait 30s reconnect window, then remove.

### Game Server — Horizontal

**When to scale:**
- Entities per zone > 100.
- Zones per Game Server > 50.
- CPU > 70% for 30s.
- Memory > 80% for 30s.
- Zone imbalance (stddev > 30%) — rebalance, do not add server.

**How to scale:**

1. Room Service detects threshold breach.
2. Room Service signals orchestrator to spawn new Game Server.
3. New instance registers with Room Service (`JOINING` state).
4. Room Service selects most loaded ACTIVE servers.
5. Room Service transfers zones (least loaded first, ~1-2s per zone).
6. New server reaches `ACTIVE` state.

**Scale-down:**

1. Room Service selects DRAINING candidate (fewest zones).
2. All zones transferred to other ACTIVE servers.
3. Candidate transitions to SHUTDOWN and exits.

**Never** kill a Game Server with active zones. Always drain first.

### Room Service — Vertical

Room Service is an active/passive pair. The leader handles zone assignment, load balancing, and service discovery. The follower is a warm standby.

**When to scale:**
- Zone lookups > 10,000 req/s.
- Game Server registrations > 100.
- CPU > 70% for 30s.

**How to scale:**
- Increase CPU/memory allocation to the Room Service pod.
- If load exceeds single-node capacity, add read-only replicas for zone lookup queries (PostgreSQL handles the reads).

**Why not horizontal:** Room Service uses leader election (Kubernetes Lease API). Only one leader makes coordination decisions. Multiple leaders would require distributed consensus (unnecessary complexity for current scale).

### PostgreSQL — Vertical (Primary) / Horizontal (Read Replicas)

**When to scale:**
- Concurrent connections > 40 (80% of pool).
- Write throughput > 1,000 zone ownership writes/s.
- Query latency p99 > 50ms.

**How to scale vertically:**
- Increase CPU/RAM on the PostgreSQL node/pod.
- Increase `max_connections` and shared buffers.

**How to scale horizontally (read replicas):**
1. Provision replica via streaming replication.
2. Configure Room Service / Gateway to route read queries to replica.
3. Writes still hit primary.

**Future:** PgBouncer for connection pooling when connections exceed 50.

### Redis — Vertical (Primary) / Horizontal (Cluster)

**When to scale:**
- Memory > 80% of `maxmemory`.
- Session cache entries > 100,000.
- Metadata cache keys > 10,000.

**How to scale vertically:**
- Increase `maxmemory` and pod resources.
- 100,000 session entries × ~100 bytes = ~10 MB. Currently negligible.

**How to scale horizontally (future):**
- Redis Cluster shards keys across nodes.
- Requires application-level awareness (go-redis cluster client).

**Current recommendation:** Vertical scaling for Redis is sufficient for MVP. Plan for Redis Cluster when session count exceeds 100K.

## Pre-Scaling for Known Events

| Event | Action | Timing |
|-------|--------|--------|
| Game launch / event | Double Game Server replicas | 60 min before |
| Marketing campaign | Pre-scale Gateway + Game Server | 30 min before |
| Load test | Scale to target replica count | 15 min before |
| Regional holiday | Expected load increase % | 24 hours before |

Pre-scaling steps:

1. Recalculate capacity: expected peak connections / players / entities.
2. Set target replica counts based on capacity thresholds.
3. Apply via Helm values override or K8s HPA min replicas override.
4. Monitor: verify new replicas register and pass health checks.
5. After event: gradually reduce to normal levels.

**Example pre-scale for game launch (10,000 concurrent players):**

```
Expected entities: 10,000 players × 1 entity = 10,000 entities
Game Server capacity: 5,000 entities per server
Required Game Servers: 3 (2 + 1 buffer)

Pre-scale Gateways:
Expected connections: 10,000
Gateway capacity: 10,000 per instance
Required Gateways: 2 (1 + 1 buffer)
```

## Capacity Planning Verification

Verify capacity assumptions regularly via benchmarks (see [ADR-020](../adr/020-benchmark-strategy.md)):

| Check | Frequency | Method |
|-------|-----------|--------|
| Gateway connections | Per release | Load test with N concurrent WebSocket connections |
| Game Server entity capacity | Per release | Benchmark with N entities per zone |
| PostgreSQL write throughput | Monthly | pgbench with zone ownership workload |
| Redis memory usage | Weekly | `redis-cli INFO memory` + `MEMORY STATS` |
| End-to-end latency | Per release | Simulated client load with p99 latency measurement |

### Verification Procedure

```bash
# Gateway connection test (staging)
k6 run test/load/gateway-connections.js \
  --vus 10000 --duration 5m

# Game Server entity benchmark
go test ./test/bench/... -bench=BenchmarkEntityThroughput \
  -benchtime=5x

# PostgreSQL write benchmark
pgbench -h localhost -U spatial -d spatialdb \
  -f test/load/zone-ownership.sql \
  -c 20 -T 300
```

If benchmarks show capacity below targets defined in [ADR-017](../adr/017-capacity-planning.md), investigate bottlenecks before deploying to production.

## Vertical vs. Horizontal Decision Matrix

| Condition | Vertical | Horizontal |
|-----------|----------|------------|
| Can service be replicated? | — | Yes (Gateway, Game Server) |
| Does service hold exclusive state? | Yes (Room Service leader) | No |
| Is service database-backed? | Yes (PostgreSQL, Redis) | Read replicas only |
| Cost of adding replica | Low (config change) | Medium (new instance + zone transfer) |
| Scaling latency | Seconds (restart) | Minutes (spawn + register + zone transfer) |

## References

- [ADR-007](../adr/007-autoscaling.md) — Autoscaling
- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning
- [ADR-015](../adr/015-architecture-principles.md) — Architecture Principles
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [Runbook](runbook.md)
- [Deployment Guide](deployment.md)
