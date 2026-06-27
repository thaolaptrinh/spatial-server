# Scaling Strategy

> **Last Updated:** 2026-06-26

## Purpose

Defines how each service scales horizontally and vertically, what triggers scaling actions, and the flow of scale-up and scale-down operations.

## Per-Service Strategy

### Gateway — Horizontal, Stateless

| Property | Detail |
|----------|--------|
| **Strategy** | Add/remove instances behind load balancer |
| **State** | Fully stateless — no session affinity needed |
| **Health check** | `/health` (TCP or HTTP) |
| **Routing** | Client routed by Room Service lookup at connect time |
| **Per-instance limit** | 10,000 concurrent WebSocket connections |
| **Scale trigger** | CPU >70% for 30s OR connections >8,000 for 30s |
| **Scale-out action** | Deploy new Gateway pod, register with LB |
| **Scale-in action** | Drain connections (stop accepting new, wait for active to drop), remove from LB, terminate |

**Design note:** Gateway is the easiest service to scale — stateless means zero coordination. The bottleneck is file descriptors and goroutine overhead per connection.

### Room Service — High Availability, Leader Election

| Property | Detail |
|----------|--------|
| **Strategy** | Active/Passive pair with leader election |
| **Election mechanism** | K3s (Kubernetes) Lease API (`coordination.k8s.io`) in production; single instance in dev |
| **Replicas** | 2 (production) |
| **Failover time** | <5 seconds |
| **Leader handles** | Zone assignment, load balancing decisions, service discovery writes |
| **Follower role** | Warm standby — accepts readiness probes, can serve read-only lookups |
| **Scaling** | Room Service is not expected to scale beyond 2 replicas. It processes metadata only — 100 Game Servers, 1,000 runtimes, 10K lookups/s. If throughput exceeds capacity, add read replicas (follower can serve `LookupZone`). |

**Design note:** Room Service is NOT a bottleneck by design (see ADR-004). If it becomes one, investigate the load pattern before adding replicas.

### Game Server — Horizontal, Coordinator-Managed

| Property | Detail |
|----------|--------|
| **Strategy** | Room Service decides when to add/remove Game Server instances |
| **State** | Stateful — owns zones with in-memory entity + AOI state |
| **Per-instance limit** | 5,000 entities (100/zone × 50 zones) |
| **Scale trigger** | CPU >70% for 30s OR memory >80% for 30s OR zone imbalance stddev >30% |
| **Scale-out action** | Spawn new Game Server → Room Service registers it → zones transferred from overloaded servers |
| **Scale-in action** | Room Service selects drain candidate → all zones transferred → server terminates |

#### Scale-Up Flow

```
1. Room Service detects threshold breach (CPU >70%, memory >80%, or zone stddev >30%)
2. Room Service signals orchestrator to spawn new Game Server
   (Dev: docker compose scale; Prod: HPA custom metric → pod spawn)
3. New Game Server starts → connects to Room Service → Register(JOINING)
4. Room Service selects most loaded ACTIVE Game Servers
5. Room Service picks zones to transfer from each (least loaded zones first)
6. For each zone: PREPARE (source pauses AOI updates) → stream snapshot via gRPC
7. Target loads snapshot, starts simulation, confirms ownership
8. Room Service updates zone_ownership table (PostgreSQL transaction)
9. Gateway routing table refreshed (pushed or polled)
10. New Game Server reaches ACTIVE state
```

#### Scale-Down Flow

```
1. Room Service detects sustained low load (all metrics below 30% for 5 min)
2. Room Service selects drain candidate (fewest zones, lowest load)
3. Room Service sends PrepareShutdown to candidate
4. Candidate rejects new zone assignments
5. Room Service transfers all zones from candidate to other servers
6. Candidate reaches 0 zones → transitions to SHUTDOWN
7. Room Service deregisters candidate
8. Candidate terminates
```

#### Game Server Crash Recovery

```
1. Room Service detects heartbeat timeout (3 consecutive misses = 15s)
2. Room Service marks Game Server as LOST in zone_ownership table
3. Room Service identifies zones owned by lost server
4. Room Service assigns orphan zones to available Game Servers
5. New owners load zone state from PostgreSQL (last persisted snapshot)
6. Ghost entities cleaned on next AOI sweep
7. Gateway routing table updated
```

### Redis — Standalone → Sentinel → Cluster

| Environment | Topology |
|-------------|----------|
| Local Dev | Standalone single instance |
| Staging | Sentinel (1 primary, 2 replicas) |
| Production | Redis Cluster (data sharded across nodes) |

### PostgreSQL — Primary + Replicas

| Environment | Topology |
|-------------|----------|
| Local Dev | Single instance |
| Staging | Primary + 1 replica (reads) |
| Production | Primary + N replicas (read replicas for analytics) |

Write path always goes to primary. Read replicas used for analytics queries only — hot-path reads use primary (to avoid replication lag issues).

## Autoscaling Thresholds

| Metric | Threshold | Duration | Action |
|--------|-----------|----------|--------|
| Gateway CPU | >70% | 30s | Add Gateway replica |
| Gateway connections | >8,000 (80% of 10K) | 30s | Add Gateway replica |
| Game Server CPU | >70% | 30s | Add Game Server replica |
| Game Server memory | >80% | 30s | Add Game Server replica |
| Zone ownership imbalance | stddev >30% | Immediate | Rebalance zones (no new server) |
| PostgreSQL connections | >40 (80% of pool) | 30s | Add PgBouncer or increase pool |
| Redis memory | >80% of maxmemory | 30s | Scale up or enable eviction |

## Scale-Down Safety

- Game Server with active zones is **never** killed
- Scale-down selects drain candidates with the fewest zones first
- Zones are transferred one at a time with state snapshot + confirmation
- PostgresSQL acts as tiebreaker for ownership during network partitions (advisory lock)

## References

- [ADR-007](../adr/007-autoscaling.md) — Autoscaling Strategy
- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning (per-service limits)
- [ADR-002](../adr/002-zone-migration.md) — Zone Migration (zone transfer protocol)
- [ADR-004](../adr/004-coordinator.md) — Coordinator Pattern (Room Service's role)
- [Overview](overview.md) — Component responsibilities
