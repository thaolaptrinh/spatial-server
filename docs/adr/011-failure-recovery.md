# ADR 011: Failure Recovery

## Status

Approved

## Context

Distributed systems experience failures. The platform must define recovery behavior for each component failure scenario.

## Problem

In a distributed spatial system, failures are inevitable. Each component (Gateway, Room Service, Game Server, Redis, PostgreSQL) has different failure modes requiring specific recovery strategies to minimize player impact.

## Decision

### Gateway Crash

- **Detection**: Health check fails, LB removes from rotation.
- **Recovery**: LB routes to other Gateway instances.
- **Client impact**: Disconnected clients reconnect via session token (30s reconnect window).
- **State loss**: In-memory session data lost. Redis-backed session can resume.

### Room Service Crash

- **Development**: Single instance → manual restart. Gateway cache still valid for 5s TTL.
- **Production**: K8s Lease API → follower becomes leader within seconds.
- **Recovery**: New leader reads ownership table from PostgreSQL. Gateway cache bridges the gap.
- **Client impact**: No gameplay impact during failover (direct P2P RPCs continue, only zone lookups affected).

### Game Server Crash

- **Detection**: Heartbeat timeout (15s).
- **Recovery**:
  1. Room Service marks zones as ORPHAN.
  2. Room Service assigns zones to ACTIVE servers.
  3. New owners load zone state from PostgreSQL (last persisted state).
  4. Clients in affected zones reconnect → routed to new owner via Gateway.
- **Data loss**: In-memory state since last persistence interval (configurable, default 5s).
- **Client impact**: Visible interruption (entity positions snap to last persisted state).

### Redis Crash

- **Detection**: Connection errors from go-redis.
- **Recovery**: Graceful degradation — reads hit PostgreSQL directly (slower but correct).
- **Impact**: Performance degradation, no data loss.
- **On restart**: Redis repopulates from PostgreSQL (cache warming).

### PostgreSQL Crash

- **Detection**: Connection errors from pgx.
- **Recovery**: All services operate in degraded mode:
  - Reads from in-memory/Redis cache only.
  - Writes queued (bounded buffer, drop oldest on overflow).
  - Zone ownership changes blocked (no new transfers).
- **On recovery**: Replay queued writes, resume normal operation.
- **Impact**: No new zone transfers. Existing gameplay continues (in-memory state). If crash exceeds buffer capacity, data loss may occur.

### Network Partition

- **Detection**: Heartbeat timeouts on both sides of partition.
- **Split-brain prevention**:
  1. Room Service detects heartbeat timeout from Server A (zone Z owner).
  2. Room Service marks zone Z as LOST.
  3. Room Service assigns zone Z to Server B.
  4. Server A's heartbeat eventually returns → A discovers it no longer owns Z.
  5. A surrenders zone Z, cleans up local state.
- **PostgreSQL** is the tiebreaker (advisory lock for ownership transactions).
- **Risk window**: ~15s (3 missed heartbeats) where stale reads are possible.

## Alternatives

1. **Full replication (active-active)**: Every component runs active-active with complete state replication. Highest availability but highest complexity, cost, and coordination overhead.
2. **Crash-only design**: No graceful shutdown procedures — always recover from persistent state. Simpler but increases recovery time and potential data loss.
3. **State checkpointing to object store**: Periodically checkpoint zone state to S3/GCS. More durable for long-term recovery but adds latency and storage cost.

## Tradeoffs

- 15s heartbeat timeout balances false positive tolerance against detection speed for genuine failures.
- PostgreSQL crash is the most severe scenario — queued writes may overflow the bounded buffer.
- 5s persistence interval trades potential data loss for write throughput — acceptable for MVP gameplay.

## Consequences

- Best-effort consistency during failures. Eventual consistency within ~15s.
- PostgreSQL crash is the most severe scenario (writes queued, may lose data).
- Redis crash is mild (just slower reads).
- Game Server crash loses up to 5s of in-memory state (acceptable for MVP).

## Future Considerations

- Automated chaos engineering tests for failure scenarios.
- Multi-region failover for disaster recovery.
- State checkpointing to S3 for long-term durability.

## Replaces

None.
