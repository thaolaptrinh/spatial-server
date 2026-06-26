# ADR 001: Zone Ownership

## Status

Approved

## Context

A runtime's spatial area is divided into grid cells called zones. Each zone must be owned by exactly one Game Server at any time to ensure consistency. We need a mechanism to assign, transfer, and recover zone ownership.

## Problem

Zone ownership conflicts can cause split-brain scenarios, data corruption, and inconsistent gameplay. Without a clear ownership protocol, multiple servers might claim the same zone, or orphan zones may remain unassigned after server failures.

## Decision

- Zone ownership is stored in PostgreSQL: `zone_ownership(zone_id, server_id, status, heartbeat_expires_at)`.
- Room Service is the authority for ownership decisions (which zone goes to which server).
- Only Room Service writes to the ownership table.
- Game Servers read ownership (via Room Service lookup or cached).
- Ownership is claimed via PostgreSQL INSERT (unique constraint prevents double-claim).
- Ownership transfer is two-phase: PREPARE (pause zone) → COMMIT (new owner starts).
- Recover orphan zones after heartbeat timeout (3 missed heartbeats = 15s).

## Alternatives

1. **Consensus-based (Raft/etcd)**: Use distributed consensus for ownership decisions. Higher complexity and latency but stronger guarantees across partitions.
2. **Redis-based leasing**: Use Redis SETNX with TTL for lightweight ownership. Less durable than PostgreSQL — Redis restart loses lease state.
3. **CRDT-based**: Allow multiple writers and resolve conflicts via CRDTs. Higher implementation complexity for marginal gain in this domain.

## Tradeoffs

- PostgreSQL approach trades absolute availability for strong consistency via unique constraints.
- Heartbeat-based timeout (15s) means brief unavailability before orphan recovery begins.
- Two-phase transfer introduces operational complexity but ensures atomic ownership changes.

## Consequences

- PostgreSQL unique constraint guarantees no split-brain for zone ownership.
- Room Service is the single writer — simple consistency model.
- Zone transfer requires brief pause (tens of ms).
- Gateway must cache ownership for low-latency routing (TTL: 5s).

## Future Considerations

- Sharded Room Service for write throughput at scale.
- Lease-based ownership with shorter timeouts for faster failover in production.
- Read-only replicas for ownership lookups to reduce PostgreSQL load.

## Replaces

None.
